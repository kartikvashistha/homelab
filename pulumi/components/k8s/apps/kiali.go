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

type KialiArgs struct {
	Hostnames []string `json:"hostnames"`
	// PrometheusURL is a Prometheus-compatible query endpoint for Istio
	// metrics, e.g. Mimir's http://mimir-query-frontend.observability.svc:8080/prometheus.
	// Empty keeps Kiali's default (http://prometheus.istio-system:9090).
	PrometheusURL string `json:"prometheusUrl,omitempty"`
	// PrometheusHealthCheckURL overrides the URL Kiali probes to decide
	// Prometheus is up. Needed for Mimir, which has no <url>/-/healthy and
	// answers 404; use its /ready. Empty keeps Kiali's default.
	PrometheusHealthCheckURL string `json:"prometheusHealthCheckUrl,omitempty"`
	// GrafanaInternalURL is how Kiali reaches Grafana from inside the cluster,
	// e.g. http://grafana.observability.svc:80. Empty leaves Kiali's Grafana
	// integration at its default (an unreachable grafana.istio-system:3000).
	GrafanaInternalURL string `json:"grafanaInternalUrl,omitempty"`
	// GrafanaExternalURL is the address browsers use for the links Kiali shows,
	// e.g. https://grafana.internal. Only used with GrafanaInternalURL.
	GrafanaExternalURL string `json:"grafanaExternalUrl,omitempty"`
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
	opts ...pulumi.ResourceOption,
) (*KialiComponent, error) {
	comp := &KialiComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:Kiali", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register kiali component: %w", err)
	}
	parentOpt := pulumi.Parent(comp)
	crSpec := pulumi.Map{
		// Anonymous access: acceptable behind the internal gateway
		// only. Revisit before exposing outside the LAN.
		"auth": pulumi.Map{"strategy": pulumi.String("anonymous")},
	}
	externalServices := pulumi.Map{}
	if args.PrometheusURL != "" {
		prom := pulumi.Map{"url": pulumi.String(args.PrometheusURL)}
		if args.PrometheusHealthCheckURL != "" {
			prom["health_check_url"] = pulumi.String(args.PrometheusHealthCheckURL)
		}
		externalServices["prometheus"] = prom
	}
	if args.GrafanaInternalURL != "" {
		grafana := pulumi.Map{
			"enabled":      pulumi.Bool(true),
			"internal_url": pulumi.String(args.GrafanaInternalURL),
		}
		if args.GrafanaExternalURL != "" {
			grafana["external_url"] = pulumi.String(args.GrafanaExternalURL)
		}
		externalServices["grafana"] = grafana
	}
	if len(externalServices) > 0 {
		crSpec["external_services"] = externalServices
	}
	operatorNamespace, err := corev1.NewNamespace(ctx, name+"-operator-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(kialiOperatorNamespace)},
	}, parentOpt)
	if err != nil {
		return nil, fmt.Errorf("create kiali operator namespace: %w", err)
	}

	release, err := helmv3.NewRelease(ctx, name+"-operator", &helmv3.ReleaseArgs{
		Name:           pulumi.String(kialiOperatorRelease),
		Chart:          pulumi.String("kiali-operator"),
		Namespace:      operatorNamespace.Metadata.Name(),
		Lint:           pulumi.Bool(true),
		Version:        pulumi.String(versions.KialiOperator),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(versions.KialiOperatorRepo)},
		Values: pulumi.Map{
			"cr": pulumi.Map{
				"create":    pulumi.Bool(true),
				"namespace": gateway.Namespace,
				"spec":      crSpec,
			},
		},
	}, parentOpt)
	if err != nil {
		return nil, fmt.Errorf("install kiali operator: %w", err)
	}

	if _, err := core.NewHTTPRouteComponent(ctx, name+"-route", &core.HTTPRouteArgs{
		Name:      "kiali-ui",
		Namespace: core.IstioNamespace,
		Hostnames: args.Hostnames,
		Gateway:   gateway,
		Service:   kialiServiceName,
		Port:      kialiServicePort,
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

func (KialiApp) Deploy(ctx *pulumi.Context, env Env, raw json.RawMessage) error {
	args, err := decode[KialiArgs](raw)
	if err != nil {
		return fmt.Errorf("decode kiali config: %w", err)
	}

	if len(args.Hostnames) == 0 {
		return fmt.Errorf("kiali requires at least one hostname")
	}

	if _, err := NewKialiComponent(ctx, "kiali", &args, env.Gateway); err != nil {
		return fmt.Errorf("create kiali component: %w", err)
	}

	return nil
}
