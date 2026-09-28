package k8s

import (
	// "encoding/json"

	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type GatewayComponent struct {
	pulumi.ResourceState
}

// GatewayArgs uses concrete Go types so `cfg.RequireObject` can unmarshal it
type GatewayArgs struct {
	Name      string `json:"name"`
	ClassName string `json:"className"`
	GatewayIp string `json:"gatewayIp"`
}

func NewGatewayComponent(ctx *pulumi.Context, name string, args *GatewayArgs, opts ...pulumi.ResourceOption) (*GatewayComponent, error) {
	comp := &GatewayComponent{}
	err := ctx.RegisterComponentResource("CustomComponent:k8s:Gateway", name, comp, opts...)
	if err != nil {
		return nil, err
	}

	// Unmarshal listeners struct slice into dynamic map structure
	// bytes, err := json.Marshal(args.Listeners)
	// if err != nil {
	// 	return nil, err
	// }
	// var listenersRaw []map[string]any
	// if err := json.Unmarshal(bytes, &listenersRaw); err != nil {
	// 	return nil, err
	// }

	spec := pulumi.Map{
		"gatewayClassName": pulumi.String(args.ClassName),
		// "listeners":        pulumi.ToMapArray(listenersRaw),
		"listeners": pulumi.MapArray{
			pulumi.Map{
				"name":     pulumi.String("https"),
				"protocol": pulumi.String("HTTPS"),
				"port":     pulumi.Int(443),
				"hostname": pulumi.String("*.internal"), // <-- get this from cm or something
				"allowedRoutes": pulumi.Map{
					"namespaces": pulumi.Map{
						"from": pulumi.String("All"),
					},
				},
				"tls": pulumi.Map{
					"mode": pulumi.String("Terminate"),
					"certificateRefs": pulumi.MapArray{
						pulumi.Map{
							"name": pulumi.String("homelab-internal-tls"), // <-- get this as an output from cm
						},
					},
				},
			},
		},
	}

	gatewayNs := func(s string) string {
		if s == "istio" {
			return ISTIO_NAMESPACE
		}
		return "default"
	}
	_, err = apiextensions.NewCustomResource(ctx, name+"-cr", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("Gateway"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(args.Name),
			Namespace: pulumi.String(gatewayNs(args.ClassName)),
			Annotations: pulumi.StringMap{
				"metallb.io/loadBalancerIPs":     pulumi.String(args.GatewayIp),
				"cert-manager.io/cluster-issuer": pulumi.String("internal-ca-issuer"), // <-- get this from cm output
			},
		},
		OtherFields: kpulumi.UntypedArgs{
			"spec": spec,
		},
	}, append(opts, pulumi.Parent(comp))...) // Pass parent & opts cleanly
	if err != nil {
		return nil, err
	}

	return comp, nil
}
