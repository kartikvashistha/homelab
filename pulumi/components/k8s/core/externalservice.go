package core

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	discoveryv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/discovery/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const defaultExternalServicePort = 80

// ExternalServiceConfig is the stack-config shape for a service that runs
// OUTSIDE the cluster (a NAS, a media server, a hypervisor UI, ...) but should
// be reachable through the cluster gateway under its own hostname.
//
//	k8s:externalServices:
//	  jellyfin:
//	    addresses: [192.168.50.89]
//	    port: 8899
//	    hostnames: [jellyfin.internal]
type ExternalServiceConfig struct {
	// Addresses are the IPv4 addresses of the backend(s).
	Addresses []string `json:"addresses"`
	// Port is the port the backend listens on.
	Port int `json:"port"`
	// Hostnames are routed to this service by the gateway.
	Hostnames []string `json:"hostnames"`
	// Namespace to create for the Service and HTTPRoute. Defaults to the
	// component name.
	Namespace string `json:"namespace,omitempty"`
	// ServicePort is the port exposed by the in-cluster Service. Defaults to 80.
	ServicePort int `json:"servicePort,omitempty"`
}

// Validate checks the config without touching Pulumi, so mistakes fail early
// (during preview, before any resource is registered).
func (c *ExternalServiceConfig) Validate() error {
	if len(c.Hostnames) == 0 {
		return errors.New("at least one hostname is required")
	}
	if len(c.Addresses) == 0 {
		return errors.New("at least one backend address is required")
	}
	for _, a := range c.Addresses {
		// The EndpointSlice below is IPv4-only, and hostnames are not valid
		// EndpointSlice addresses, so reject anything else up front.
		if !isIPv4(a) {
			return fmt.Errorf("address %q is not a valid IPv4 address", a)
		}
	}
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("port must be between 1 and 65535, got %d", c.Port)
	}
	if c.ServicePort < 0 || c.ServicePort > 65535 {
		return fmt.Errorf("servicePort must be between 1 and 65535, got %d", c.ServicePort)
	}
	return nil
}

type ExternalServiceArgs struct {
	ExternalServiceConfig
	Gateway *GatewayComponent
}

type ExternalServiceComponent struct {
	pulumi.ResourceState

	Namespace   pulumi.StringOutput
	ServiceName pulumi.StringOutput
}

// NewExternalServiceComponent exposes an out-of-cluster backend through the
// gateway. It creates:
//
//	Namespace -> selector-less Service + EndpointSlice -> HTTPRoute
//
// The Service has no selector, so Kubernetes does not manage its endpoints;
// the hand-written EndpointSlice (linked via the kubernetes.io/service-name
// label) points it at the external addresses instead.
func NewExternalServiceComponent(
	ctx *pulumi.Context,
	name string,
	args *ExternalServiceArgs,
	opts ...pulumi.ResourceOption,
) (*ExternalServiceComponent, error) {
	if args.Gateway == nil {
		return nil, fmt.Errorf("external service %q: gateway is required", name)
	}
	if err := args.Validate(); err != nil {
		return nil, fmt.Errorf("external service %q: %w", name, err)
	}

	namespace := cmp.Or(args.Namespace, name)
	servicePort := cmp.Or(args.ServicePort, defaultExternalServicePort)
	serviceName := name + "-svc"

	gatewayDep := pulumi.DependsOn([]pulumi.Resource{args.Gateway})

	comp := &ExternalServiceComponent{}
	if err := ctx.RegisterComponentResource(
		"homelab:k8s:ExternalService",
		name,
		comp,
		// slices.Concat copies; a bare append(opts, ...) could write into the
		// caller's backing array.
		slices.Concat(opts, []pulumi.ResourceOption{
			gatewayDep,
		})...,
	); err != nil {
		return nil, fmt.Errorf("register external service %q: %w", name, err)
	}

	// Parent establishes ownership in the resource tree; DependsOn
	// establishes ordering. Children need both.
	childOpts := []pulumi.ResourceOption{pulumi.Parent(comp), gatewayDep}

	ns, err := corev1.NewNamespace(ctx, name+"-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(namespace),
		},
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create namespace for %q: %w", name, err)
	}

	endpointSlice, err := discoveryv1.NewEndpointSlice(ctx, name+"-endpoint-slice", &discoveryv1.EndpointSliceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(serviceName),
			Namespace: ns.Metadata.Name(),
			Labels: pulumi.StringMap{
				"kubernetes.io/service-name": pulumi.String(serviceName),
			},
		},
		AddressType: pulumi.String("IPv4"),
		Ports: discoveryv1.EndpointPortArray{
			discoveryv1.EndpointPortArgs{
				Name:     pulumi.String("http"),
				Protocol: pulumi.String("TCP"),
				Port:     pulumi.Int(args.Port),
			},
		},
		Endpoints: discoveryv1.EndpointArray{
			discoveryv1.EndpointArgs{
				Addresses: pulumi.ToStringArray(args.Addresses),
			},
		},
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create endpoint slice for %q: %w", name, err)
	}

	svc, err := corev1.NewService(ctx, name+"-svc", &corev1.ServiceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(serviceName),
			Namespace: ns.Metadata.Name(),
		},
		Spec: &corev1.ServiceSpecArgs{
			Ports: &corev1.ServicePortArray{
				corev1.ServicePortArgs{
					Name:       pulumi.String("http"),
					Protocol:   pulumi.String("TCP"),
					Port:       pulumi.Int(servicePort),
					TargetPort: pulumi.Int(args.Port),
				},
			},
		},
	}, childOpts...)
	if err != nil {
		return nil, fmt.Errorf("create service for %q: %w", name, err)
	}

	// The route's namespace is a plain string, so Pulumi can't infer that it
	// must wait for the namespace; say so explicitly.
	routeOpts := slices.Concat(childOpts, []pulumi.ResourceOption{
		pulumi.DependsOn([]pulumi.Resource{ns, svc, endpointSlice}),
	})
	if _, err := NewHTTPRouteComponent(ctx, name+"-http-route", &HTTPRouteArgs{
		Name:      name + "-route",
		Namespace: namespace,
		Hostnames: args.Hostnames,
		Gateway:   args.Gateway,
		Service:   serviceName,
		Port:      servicePort,
	}, routeOpts...); err != nil {
		return nil, fmt.Errorf("create http route for %q: %w", name, err)
	}

	comp.Namespace = ns.Metadata.Name().Elem()
	comp.ServiceName = svc.Metadata.Name().Elem()

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{
		"namespace":   comp.Namespace,
		"serviceName": comp.ServiceName,
	}); err != nil {
		return nil, err
	}

	return comp, nil
}
