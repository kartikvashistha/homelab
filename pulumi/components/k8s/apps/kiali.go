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
				"spec": pulumi.Map{
					// Anonymous access: acceptable behind the internal gateway
					// only. Revisit before exposing outside the LAN.
					"auth": pulumi.Map{"strategy": pulumi.String("anonymous")},
				},
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

func (KialiApp) Deploy(ctx *pulumi.Context, gateway *core.GatewayComponent, raw json.RawMessage) error {
	args, err := decode[KialiArgs](raw)
	if err != nil {
		return fmt.Errorf("decode kiali config: %w", err)
	}

	if len(args.Hostnames) == 0 {
		return fmt.Errorf("kiali requires at least one hostname")
	}

	if _, err := NewKialiComponent(ctx, "kiali", &args, gateway); err != nil {
		return fmt.Errorf("create kiali component: %w", err)
	}

	return nil
}
