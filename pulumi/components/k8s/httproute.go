package k8s

import (
	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type HTTPRouteComponent struct {
	pulumi.ResourceState
}

type HTTPRouteArgs struct {
	Name      string
	Namespace string
	Hostname  string
	Gateway   *GatewayComponent
	Service   string
	Port      int
}

func NewHTTPRouteComponent(
	ctx *pulumi.Context,
	name string,
	args *HTTPRouteArgs,
	opts ...pulumi.ResourceOption,
) (*HTTPRouteComponent, error) {
	comp := &HTTPRouteComponent{}

	if err := ctx.RegisterComponentResource(
		"CustomComponent:k8s:HTTPRoute",
		name,
		comp,
		opts...,
	); err != nil {
		return nil, err
	}

	spec := pulumi.Map{
		"parentRefs": pulumi.MapArray{
			pulumi.Map{
				"name":      args.Gateway.Name,
				"namespace": args.Gateway.Namespace,
			},
		},
		"hostnames": pulumi.StringArray{
			pulumi.String(args.Hostname),
		},
		"rules": pulumi.MapArray{
			pulumi.Map{
				"matches": pulumi.MapArray{
					pulumi.Map{
						"path": pulumi.Map{
							"type":  pulumi.String("PathPrefix"),
							"value": pulumi.String("/"),
						},
					},
				},
				"backendRefs": pulumi.MapArray{
					pulumi.Map{
						"name": pulumi.String(args.Service),
						"port": pulumi.Int(args.Port),
					},
				},
			},
		},
	}

	_, err := apiextensions.NewCustomResource(
		ctx,
		name+"-cr",
		&apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
			Kind:       pulumi.String("HTTPRoute"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(args.Name),
				Namespace: pulumi.String(args.Namespace),
			},
			OtherFields: kpulumi.UntypedArgs{
				"spec": spec,
			},
		},
		append(opts, pulumi.Parent(comp))...,
	)
	if err != nil {
		return nil, err
	}

	return comp, nil
}
