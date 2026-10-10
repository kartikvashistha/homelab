package apps

import (
	"fmt"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	KIALI_OPERATOR_RELEASE_NAME = "kiali-operator"
	KIALI_OPERATOR_CHART_NAME   = "kiali-operator"
	KIALI_OPERATOR_NAMESPACE    = "kiali-operator"
	KIALI_OPERATOR_REPO         = "https://kiali.org/helm-charts"
	KIALI_OPERATOR_VERSION      = "2.32.0"

	KIALI_SERVICE      = "kiali"
	KIALI_SERVICE_PORT = 20001
)

type KialiArgs struct {
	Hostnames []string `json:"hostnames"`
}

type KialiComponent struct {
	pulumi.ResourceState
}

func NewKialiComponent(ctx *pulumi.Context, name string, args *KialiArgs, gateway *core.GatewayComponent, opts ...pulumi.ResourceOption) (*KialiComponent, error) {
	comp := &KialiComponent{}

	if err := ctx.RegisterComponentResource(
		"CustomComponent:k8sApps:Kiali", name, comp, opts...); err != nil {
		return nil, err
	}

	operatorNamespace, err := corev1.NewNamespace(ctx, name+"-operator-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(KIALI_OPERATOR_NAMESPACE),
		},
	},
		pulumi.Parent(comp),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create Kiali operator namespace: %w",
			err,
		)
	}

	_, err = helmv3.NewRelease(ctx, KIALI_OPERATOR_RELEASE_NAME, &helmv3.ReleaseArgs{
		Chart: pulumi.String(KIALI_OPERATOR_CHART_NAME),

		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String(KIALI_OPERATOR_REPO),
		},

		Name:      pulumi.String(KIALI_OPERATOR_RELEASE_NAME),
		Namespace: operatorNamespace.Metadata.Name(),
		Version:   pulumi.String(KIALI_OPERATOR_VERSION),

		Values: pulumi.Map{
			"cr": pulumi.Map{
				"create": pulumi.Bool(true),

				"namespace": gateway.Namespace,

				"spec": pulumi.Map{
					"auth": pulumi.Map{
						"strategy": pulumi.String("anonymous"),
					},
				},
			},
		},
	},
		pulumi.Parent(comp),
		pulumi.DependsOn([]pulumi.Resource{operatorNamespace}),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create Kiali operator Helm release: %w",
			err,
		)
	}

	_, err = core.NewHTTPRouteComponent(ctx, name+"-http-route", &core.HTTPRouteArgs{
		Name:      "kiali-ui",
		Namespace: gateway.Namespace,
		Hostnames: args.Hostnames,
		Gateway:   gateway,
		Service:   KIALI_SERVICE,
		Port:      KIALI_SERVICE_PORT,
	},
		pulumi.Parent(comp),
		pulumi.DependsOn([]pulumi.Resource{gateway}),
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create Kiali HTTPRoute: %w",
			err,
		)
	}

	return comp, nil
}

type KialiApp struct{}

func (KialiApp) Name() string {
	return "kiali"
}

func (KialiApp) Deploy(ctx *pulumi.Context, gateway *core.GatewayComponent, args any) error {
	appCfg, ok := args.(KialiArgs)
	if !ok {
		return fmt.Errorf("invalid config for kiali")
	}

	if len(appCfg.Hostnames) == 0 {
		return fmt.Errorf("kiali hostname is required")
	}

	_, err := NewKialiComponent(ctx, "kiali", &appCfg, gateway)
	if err != nil {
		return fmt.Errorf("create kiali component: %w", err)
	}

	return nil
}
