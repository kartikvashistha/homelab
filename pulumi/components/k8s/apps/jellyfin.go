package apps

import (
	"fmt"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	discoveryv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/discovery/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type JellyfinComponent struct {
	pulumi.ResourceState
}

type JellyfinArgs struct {
	Hostnames  []string `json:"hostnames"`
	TargetPort int      `json:"targetPort"`
	Address    []string `json:"address"`
}

const (
	JELLYFIN_NAMESPACE string = "jellyfin"
	JELLYFIN_SVC_NAME  string = "jellyfin-svc"
	JELLYFIN_SVC_PORT         = 80

	HTTP pulumi.String = pulumi.String("http")
	TCP  pulumi.String = pulumi.String("TCP")
)

func NewJellyfinComponent(
	ctx *pulumi.Context,
	name string,
	args *JellyfinArgs,
	gateway *core.GatewayComponent,
	opts ...pulumi.ResourceOption,
) (*JellyfinComponent, error) {
	comp := &JellyfinComponent{}

	// The component itself depends on both core resources.
	componentOpts := append(
		opts,
		pulumi.DependsOn([]pulumi.Resource{
			gateway,
		}),
	)

	if err := ctx.RegisterComponentResource(
		"CustomComponent:k8sApps:Jellyfin",
		name,
		comp,
		componentOpts...,
	); err != nil {
		return nil, err
	}

	// Explicit dependencies are also applied to the child resources.
	//
	// Parent establishes ownership in the Pulumi tree; DependsOn
	// establishes the actual resource ordering.
	childOpts := []pulumi.ResourceOption{
		pulumi.Parent(comp),
		pulumi.DependsOn([]pulumi.Resource{
			gateway,
		}),
	}

	// ------------------------------------------------------------
	// Namespace
	// ------------------------------------------------------------

	ns, err := corev1.NewNamespace(
		ctx,
		name+"-ns",
		&corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(JELLYFIN_NAMESPACE),
			},
		},
		childOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create jellyfin namespace: %w",
			err,
		)
	}

	// ------------------------------------------------------------
	// EndpointSlice
	// ------------------------------------------------------------

	es, err := discoveryv1.NewEndpointSlice(
		ctx,
		name+"-endpoint-slice",
		&discoveryv1.EndpointSliceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(JELLYFIN_SVC_NAME),
				Namespace: ns.Metadata.Name(),
				Labels: pulumi.StringMap{
					"kubernetes.io/service-name": pulumi.String(JELLYFIN_SVC_NAME),
				},
			},

			AddressType: pulumi.String("IPv4"),

			Ports: discoveryv1.EndpointPortArray{
				discoveryv1.EndpointPortArgs{
					Name:     HTTP,
					Protocol: TCP,
					Port:     pulumi.Int(args.TargetPort),
				},
			},

			Endpoints: discoveryv1.EndpointArray{
				discoveryv1.EndpointArgs{
					Addresses: pulumi.ToStringArray(
						args.Address,
					),
				},
			},
		},
		childOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create jellyfin endpoint slice: %w",
			err,
		)
	}

	// ------------------------------------------------------------
	// Service
	// ------------------------------------------------------------

	svc, err := corev1.NewService(
		ctx,
		name+"-svc",
		&corev1.ServiceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name:      es.Metadata.Name(),
				Namespace: ns.Metadata.Name(),
			},

			Spec: &corev1.ServiceSpecArgs{
				Ports: &corev1.ServicePortArray{
					corev1.ServicePortArgs{
						Name:       HTTP,
						Protocol:   TCP,
						Port:       pulumi.Int(JELLYFIN_SVC_PORT),
						TargetPort: pulumi.Int(args.TargetPort),
					},
				},
			},
		},
		childOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create jellyfin service: %w",
			err,
		)
	}

	// ------------------------------------------------------------
	// HTTPRoute
	// ------------------------------------------------------------

	_, err = core.NewHTTPRouteComponent(ctx, name+"-http-route", &core.HTTPRouteArgs{
		Name:      name + "-route",
		Namespace: svc.Metadata.Namespace().Elem(),
		Hostnames: args.Hostnames,
		Gateway:   gateway,
		Service:   JELLYFIN_SVC_NAME,
		Port:      JELLYFIN_SVC_PORT,
	},
		childOpts...,
	)
	if err != nil {
		return nil, fmt.Errorf(
			"create jellyfin HTTP route: %w",
			err,
		)
	}

	return comp, nil
}

type JellyfinApp struct{}

func (JellyfinApp) Name() string {
	return "jellyfin"
}

func (JellyfinApp) Deploy(ctx *pulumi.Context, gateway *core.GatewayComponent, args any) error {
	appCfg, ok := args.(JellyfinArgs)
	if !ok {
		return fmt.Errorf("invalid config for jellyfin")
	}

	if len(appCfg.Hostnames) == 0 {
		return fmt.Errorf("jellyfin requires at least one hostname")
	}

	if len(appCfg.Address) == 0 {
		return fmt.Errorf("jellyfin requires at least one backend address")
	}

	if appCfg.TargetPort < 1 || appCfg.TargetPort > 65535 {
		return fmt.Errorf("jellyfin targetPort must be between 1 and 65535")
	}

	if _, err := NewJellyfinComponent(ctx, "jellyfin", &appCfg, gateway); err != nil {
		return fmt.Errorf("create jellyfin component: %w", err)
	}

	return nil
}
