package apps

import (
	"encoding/json"
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

const (
	kialiOperatorNamespace = "kiali-operator"
	kialiOperatorRelease   = "kiali-operator"
	kialiServiceName       = "kiali"
	kialiServicePort       = 20001
)

const kialiSSOClient = "kiali"

type KialiArgs struct {
	Hostnames []string `json:"hostnames"`
	// SSO switches Kiali from anonymous access to OpenID login against
	// Authentik. Requires an sso client with slug "kiali" whose redirect URI
	// is https://<hostname>/kiali.
	SSO bool `json:"sso,omitempty"`
}

type KialiComponent struct {
	pulumi.ResourceState
}

// NewKialiComponent installs the Kiali operator, has it create a Kiali CR in
// the Istio namespace, and routes Hostnames to the Kiali UI.
func NewKialiComponent(
	ctx *pulumi.Context,
	name string,
	args *KialiArgs,
	gateway *core.GatewayComponent,
	sso *core.AuthentikComponent,
	opts ...pulumi.ResourceOption,
) (*KialiComponent, error) {
	comp := &KialiComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:Kiali", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register kiali component: %w", err)
	}
	parentOpt := pulumi.Parent(comp)

	operatorNamespace, err := corev1.NewNamespace(ctx, name+"-operator-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(kialiOperatorNamespace)},
	}, parentOpt)
	if err != nil {
		return nil, fmt.Errorf("create kiali operator namespace: %w", err)
	}

	// Anonymous access is acceptable behind the internal gateway only; SSO
	// replaces it with an OpenID login.
	auth := pulumi.Map{"strategy": pulumi.String("anonymous")}
	spec := pulumi.Map{"auth": auth}
	var releaseDeps []pulumi.Resource
	if args.SSO {
		if sso == nil || !sso.HasClient(kialiSSOClient) {
			return nil, fmt.Errorf("kiali sso: no sso client %q is configured", kialiSSOClient)
		}
		clientSecret, err := sso.ClientSecret(kialiSSOClient)
		if err != nil {
			return nil, err
		}

		// Kiali reads the client secret from a Secret named "kiali" (key
		// "oidc-secret") in its own namespace, which is the Istio namespace.
		secret, err := corev1.NewSecret(ctx, name+"-oidc-secret", &corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("kiali"),
				Namespace: gateway.Namespace,
			},
			StringData: pulumi.StringMap{"oidc-secret": clientSecret},
		}, parentOpt, pulumi.DependsOn([]pulumi.Resource{gateway}))
		if err != nil {
			return nil, fmt.Errorf("create kiali oidc secret: %w", err)
		}
		releaseDeps = append(releaseDeps, secret)

		auth["strategy"] = pulumi.String("openid")
		auth["openid"] = pulumi.Map{
			"client_id":      pulumi.String(kialiSSOClient),
			"issuer_uri":     pulumi.String(sso.IssuerURL(kialiSSOClient)),
			"scopes":         pulumi.StringArray{pulumi.String("openid"), pulumi.String("profile"), pulumi.String("email")},
			"username_claim": pulumi.String("preferred_username"),
			// Kiali would otherwise need the kube-apiserver to accept
			// Authentik tokens. With this, any authenticated user gets
			// Kiali's own service-account access.
			"disable_rbac": pulumi.Bool(true),
			// TODO: mount the internal CA into Kiali and drop this; the
			// issuer is served with a cert from the self-signed internal CA.
			"insecure_skip_verify_tls": pulumi.Bool(true),
		}
		// Deliberately no server.web_fqdn / web_schema / web_port. Pinning them
		// breaks the redirect URI: with an FQDN but no port Kiali appends its
		// listen port (:20001), and web_port 443 is rendered as the character
		// U+01BB. Kiali instead derives its URL from forwarded headers, and
		// Istio's gateway doesn't send X-Forwarded-Port, so the route below
		// sets it.
	}

	var routeHeaders map[string]string
	if args.SSO {
		routeHeaders = map[string]string{"X-Forwarded-Port": "443"}
	}

	release, err := helmv3.NewRelease(ctx, name+"-operator", &helmv3.ReleaseArgs{
		Name:           pulumi.String(kialiOperatorRelease),
		Chart:          pulumi.String("kiali-operator"),
		Namespace:      operatorNamespace.Metadata.Name(),
		Version:        pulumi.String(versions.KialiOperator),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(versions.KialiOperatorRepo)},
		Values: pulumi.Map{
			"cr": pulumi.Map{
				"create":    pulumi.Bool(true),
				"namespace": gateway.Namespace,
				"spec":      spec,
			},
		},
	}, parentOpt, pulumi.DependsOn(releaseDeps))
	if err != nil {
		return nil, fmt.Errorf("install kiali operator: %w", err)
	}

	if _, err := core.NewHTTPRouteComponent(ctx, name+"-route", &core.HTTPRouteArgs{
		Name:              "kiali-ui",
		Namespace:         core.IstioNamespace,
		Hostnames:         args.Hostnames,
		Gateway:           gateway,
		Service:           kialiServiceName,
		Port:              kialiServicePort,
		SetRequestHeaders: routeHeaders,
	}, parentOpt, pulumi.DependsOn([]pulumi.Resource{gateway, release})); err != nil {
		return nil, fmt.Errorf("create kiali route: %w", err)
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{}); err != nil {
		return nil, err
	}
	return comp, nil
}

type KialiApp struct{}

func (KialiApp) Name() string { return "kiali" }

func (KialiApp) Deploy(ctx *pulumi.Context, deps Deps, raw json.RawMessage) error {
	args, err := decode[KialiArgs](raw)
	if err != nil {
		return fmt.Errorf("decode kiali config: %w", err)
	}

	if len(args.Hostnames) == 0 {
		return fmt.Errorf("kiali requires at least one hostname")
	}

	if _, err := NewKialiComponent(ctx, "kiali", &args, deps.Gateway, deps.SSO); err != nil {
		return fmt.Errorf("create kiali component: %w", err)
	}

	return nil
}
