package core

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

const (
	authentikNamespace = "authentik"
	authentikRelease   = "authentik"

	// The blueprint ConfigMap is mounted by the chart (blueprints.configMaps);
	// the Secret holds the client secrets and is loaded as env vars
	// (global.envFrom) so the blueprint can reference them with !Env.
	authentikBlueprintConfigMap = "authentik-oidc-blueprint"
	authentikOIDCSecret         = "authentik-oidc"
)

var clientSlugRe = regexp.MustCompile(`^[a-z][a-z0-9-]*[a-z0-9]$`)

// OIDCClient is one OAuth2/OIDC application registered in Authentik.
//
//	k8s:sso:
//	  clients:
//	    - slug: headlamp
//	      name: Headlamp
//	      redirectUris: ["https://headlamp.internal/oidc-callback"]
type OIDCClient struct {
	// Slug is the application slug, the OAuth client_id, and part of the
	// issuer URL (/application/o/<slug>/).
	Slug string `json:"slug"`
	// Name is the display name in the Authentik portal.
	Name string `json:"name"`
	// RedirectURIs are matched strictly.
	RedirectURIs []string `json:"redirectUris"`
	// LaunchURL is the portal tile's link (optional).
	LaunchURL string `json:"launchUrl,omitempty"`
}

// EnvVar is the environment variable carrying this client's secret.
func (c OIDCClient) EnvVar() string {
	return strings.ToUpper(strings.ReplaceAll(c.Slug, "-", "_")) + "_CLIENT_SECRET"
}

type AuthentikArgs struct {
	Hostnames []string     `json:"hostnames"`
	Clients   []OIDCClient `json:"clients"`

	// Secrets come from Pulumi config (typically a Pulumi ESC environment),
	// never from the stack's plain-text `k8s:sso` object.
	SecretKey        pulumi.StringInput            `json:"-"`
	PostgresPassword pulumi.StringInput            `json:"-"`
	ClientSecrets    map[string]pulumi.StringInput `json:"-"` // by client slug
}

func (a *AuthentikArgs) Validate() error {
	if len(a.Hostnames) == 0 {
		return errors.New("hostnames must not be empty")
	}
	if a.SecretKey == nil {
		return errors.New("secret key is required")
	}
	if a.PostgresPassword == nil {
		return errors.New("postgres password is required")
	}

	seen := map[string]bool{}
	for i, c := range a.Clients {
		switch {
		case !clientSlugRe.MatchString(c.Slug):
			return fmt.Errorf("clients[%d]: slug %q must match %s", i, c.Slug, clientSlugRe)
		case seen[c.Slug]:
			return fmt.Errorf("clients[%d]: duplicate slug %q", i, c.Slug)
		case c.Name == "":
			return fmt.Errorf("clients[%d] (%s): name is required", i, c.Slug)
		case len(c.RedirectURIs) == 0:
			return fmt.Errorf("clients[%d] (%s): at least one redirect URI is required", i, c.Slug)
		case a.ClientSecrets[c.Slug] == nil:
			return fmt.Errorf("clients[%d] (%s): no client secret provided", i, c.Slug)
		}
		seen[c.Slug] = true
	}
	return nil
}

type AuthentikComponent struct {
	pulumi.ResourceState

	hostname string
	secrets  map[string]pulumi.StringInput
}

// IssuerURL is the OIDC issuer for a client, as Authentik serves it.
func (a *AuthentikComponent) IssuerURL(slug string) string {
	return fmt.Sprintf("https://%s/application/o/%s/", a.hostname, slug)
}

// HasClient reports whether a client with this slug is registered.
func (a *AuthentikComponent) HasClient(slug string) bool {
	_, ok := a.secrets[slug]
	return ok
}

// ClientSecret returns the client secret registered for slug.
func (a *AuthentikComponent) ClientSecret(slug string) (pulumi.StringInput, error) {
	s, ok := a.secrets[slug]
	if !ok {
		return nil, fmt.Errorf("no sso client %q (add it under k8s:sso clients)", slug)
	}
	return s, nil
}

func NewAuthentikComponent(
	ctx *pulumi.Context,
	name string,
	gateway *GatewayComponent,
	args *AuthentikArgs,
	opts ...pulumi.ResourceOption,
) (*AuthentikComponent, error) {
	if err := args.Validate(); err != nil {
		return nil, fmt.Errorf("authentik %q: %w", name, err)
	}

	comp := &AuthentikComponent{
		hostname: args.Hostnames[0],
		secrets:  args.ClientSecrets,
	}
	if err := ctx.RegisterComponentResource("homelab:k8s:Authentik", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register authentik component: %w", err)
	}
	parentOpt := pulumi.Parent(comp)

	ns, err := corev1.NewNamespace(ctx, name, &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(authentikNamespace),
		},
	}, parentOpt)
	if err != nil {
		return nil, fmt.Errorf("create authentik namespace: %w", err)
	}

	values := pulumi.Map{
		"authentik": pulumi.Map{
			"secret_key": args.SecretKey,
			"error_reporting": pulumi.Map{
				"enabled": pulumi.Bool(false),
			},
			"postgresql": pulumi.Map{
				"password": args.PostgresPassword,
			},
		},
		"postgresql": pulumi.Map{
			"enabled": pulumi.Bool(true),
			"auth": pulumi.Map{
				"password": args.PostgresPassword,
			},
		},
		"server": pulumi.Map{
			"route": pulumi.Map{
				"main": pulumi.Map{
					"enabled":   pulumi.Bool(true),
					"hostnames": pulumi.ToStringArray(args.Hostnames),
					"parentRefs": pulumi.MapArray{
						pulumi.Map{
							"name":      gateway.Name,
							"namespace": gateway.Namespace,
						},
					},
				},
			},
		},
	}

	// OIDC applications are declared as an Authentik blueprint. Client
	// secrets never touch the blueprint: it reads them with !Env, and they
	// reach the pods through a Secret loaded as env vars.
	var deps []pulumi.Resource
	if len(args.Clients) > 0 {
		blueprint, err := renderBlueprint(args.Clients)
		if err != nil {
			return nil, fmt.Errorf("render authentik blueprint: %w", err)
		}

		cm, err := corev1.NewConfigMap(ctx, name+"-blueprint", &corev1.ConfigMapArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(authentikBlueprintConfigMap),
				Namespace: ns.Metadata.Name(),
			},
			Data: pulumi.StringMap{"homelab-oidc.yaml": pulumi.String(blueprint)},
		}, parentOpt)
		if err != nil {
			return nil, fmt.Errorf("create authentik blueprint: %w", err)
		}

		secretData := pulumi.StringMap{}
		for _, c := range args.Clients {
			secretData[c.EnvVar()] = args.ClientSecrets[c.Slug]
		}
		secret, err := corev1.NewSecret(ctx, name+"-oidc", &corev1.SecretArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(authentikOIDCSecret),
				Namespace: ns.Metadata.Name(),
			},
			StringData: secretData,
		}, parentOpt)
		if err != nil {
			return nil, fmt.Errorf("create authentik oidc secret: %w", err)
		}
		deps = append(deps, cm, secret)

		values["blueprints"] = pulumi.Map{
			"configMaps": pulumi.StringArray{pulumi.String(authentikBlueprintConfigMap)},
		}
		values["global"] = pulumi.Map{
			"envFrom": pulumi.MapArray{
				pulumi.Map{"secretRef": pulumi.Map{"name": pulumi.String(authentikOIDCSecret)}},
			},
		}
	}

	_, err = helmv3.NewRelease(ctx, name, &helmv3.ReleaseArgs{
		Name:           pulumi.String(authentikRelease),
		Chart:          pulumi.String(authentikRelease),
		Namespace:      ns.Metadata.Name(),
		Version:        pulumi.String(versions.Authentik),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(versions.AuthentikRepo)},
		Values:         values,
	}, parentOpt, pulumi.DependsOn(deps))
	if err != nil {
		return nil, fmt.Errorf("install authentik: %w", err)
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{}); err != nil {
		return nil, err
	}

	return comp, nil
}
