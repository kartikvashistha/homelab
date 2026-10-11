package core

import (
	"errors"
	"fmt"
	"slices"

	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// HTTPRouteArgs describes a route that sends every path under Hostnames to a
// Service, via the cluster Gateway.
type HTTPRouteArgs struct {
	Name      string
	Namespace string
	Hostnames []string
	Gateway   *GatewayComponent
	Service   string
	Port      int
}

func (a *HTTPRouteArgs) validate() error {
	switch {
	case a.Name == "":
		return errors.New("name is required")
	case a.Namespace == "":
		return errors.New("namespace is required")
	case len(a.Hostnames) == 0:
		return errors.New("at least one hostname is required")
	case a.Gateway == nil:
		return errors.New("gateway is required")
	case a.Service == "":
		return errors.New("service is required")
	case a.Port < 1 || a.Port > 65535:
		return fmt.Errorf("port must be between 1 and 65535, got %d", a.Port)
	}
	return nil
}

type HTTPRouteComponent struct {
	pulumi.ResourceState
}

func NewHTTPRouteComponent(
	ctx *pulumi.Context,
	name string,
	args *HTTPRouteArgs,
	opts ...pulumi.ResourceOption,
) (*HTTPRouteComponent, error) {
	if err := args.validate(); err != nil {
		return nil, fmt.Errorf("http route %q: %w", name, err)
	}

	comp := &HTTPRouteComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:HTTPRoute", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register http route component: %w", err)
	}

	spec := pulumi.Map{
		"parentRefs": pulumi.MapArray{
			pulumi.Map{
				"name":      args.Gateway.Name,
				"namespace": args.Gateway.Namespace,
			},
		},
		"hostnames": pulumi.ToStringArray(args.Hostnames),
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

	if _, err := apiextensions.NewCustomResource(ctx, name+"-cr", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("HTTPRoute"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(args.Name),
			Namespace: pulumi.String(args.Namespace),
		},
		OtherFields: kpulumi.UntypedArgs{"spec": spec},
	}, slices.Concat(opts, []pulumi.ResourceOption{pulumi.Parent(comp)})...); err != nil {
		return nil, fmt.Errorf("create http route %q: %w", args.Name, err)
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{}); err != nil {
		return nil, err
	}
	return comp, nil
}
