package core

import (
	"cmp"
	"errors"
	"fmt"
	"strings"

	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const defaultGatewayDomain = "internal"

type GatewayArgs struct {
	Name      string `json:"name"`
	ClassName string `json:"className"`
	GatewayIp string `json:"gatewayIp"`
	// Domain is the private DNS zone served through the gateway; the HTTPS
	// listener accepts "*.<domain>". Defaults to "internal".
	Domain string `json:"domain,omitempty"`

	// ClusterIssuer is the cert-manager ClusterIssuer that signs the gateway's
	// certificate. Set by the platform from the cert-manager component, not
	// from stack config.
	ClusterIssuer string `json:"-"`
}

// ZoneDomain returns the effective domain with the default applied and any
// leading/trailing dots removed.
func (g *GatewayArgs) ZoneDomain() string {
	return cmp.Or(strings.Trim(g.Domain, "."), defaultGatewayDomain)
}

func (g *GatewayArgs) Validate() error {
	if g.Name == "" {
		return errors.New("name is required")
	}
	if g.ClassName == "" {
		return errors.New("className is required")
	}
	if !isIPv4(g.GatewayIp) {
		return fmt.Errorf("gatewayIp %q is not a valid IPv4 address", g.GatewayIp)
	}
	return nil
}

type GatewayComponent struct {
	pulumi.ResourceState

	Name      pulumi.StringOutput
	Namespace pulumi.StringOutput
}

func gatewayNamespace(className string) string {
	if className == "istio" {
		return IstioNamespace
	}
	return "default"
}

func NewGatewayComponent(
	ctx *pulumi.Context,
	name string,
	args *GatewayArgs,
	opts ...pulumi.ResourceOption,
) (*GatewayComponent, error) {
	if err := args.Validate(); err != nil {
		return nil, fmt.Errorf("gateway %q: %w", name, err)
	}

	comp := &GatewayComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:Gateway", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register gateway component: %w", err)
	}

	annotations := pulumi.StringMap{
		"metallb.io/loadBalancerIPs": pulumi.String(args.GatewayIp),
	}
	if args.ClusterIssuer != "" {
		annotations["cert-manager.io/cluster-issuer"] = pulumi.String(args.ClusterIssuer)
	}

	spec := pulumi.Map{
		"gatewayClassName": pulumi.String(args.ClassName),
		"listeners": pulumi.MapArray{
			pulumi.Map{
				"name":     pulumi.String("https"),
				"protocol": pulumi.String("HTTPS"),
				"port":     pulumi.Int(443),
				"hostname": pulumi.String("*." + args.ZoneDomain()),
				"allowedRoutes": pulumi.Map{
					"namespaces": pulumi.Map{"from": pulumi.String("All")},
				},
				"tls": pulumi.Map{
					"mode": pulumi.String("Terminate"),
					"certificateRefs": pulumi.MapArray{
						pulumi.Map{"name": pulumi.String(args.Name + "-tls-cert")},
					},
				},
			},
		},
	}

	gateway, err := apiextensions.NewCustomResource(ctx, name+"-cr", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("Gateway"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:        pulumi.String(args.Name),
			Namespace:   pulumi.String(gatewayNamespace(args.ClassName)),
			Annotations: annotations,
		},
		OtherFields: kpulumi.UntypedArgs{"spec": spec},
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, fmt.Errorf("create gateway %q: %w", args.Name, err)
	}

	comp.Name = gateway.Metadata.Name().Elem()
	comp.Namespace = gateway.Metadata.Namespace().Elem()

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{
		"name":      comp.Name,
		"namespace": comp.Namespace,
	}); err != nil {
		return nil, err
	}
	return comp, nil
}
