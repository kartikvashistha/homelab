package k8s

import (
	"encoding/json"

	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	gapi "sigs.k8s.io/gateway-api/apis/v1"
)

type GatewayComponent struct {
	pulumi.ResourceState
}

// GatewayArgs uses concrete Go types so `cfg.RequireObject` can unmarshal it
type GatewayArgs struct {
	Name      string          `json:"name"`
	Namespace string          `json:"namespace"`
	ClassName string          `json:"className"`
	GatewayIp string          `json:"gatewayIp"`
	Listeners []gapi.Listener `json:"listeners"`
}

func NewGatewayComponent(ctx *pulumi.Context, name string, args *GatewayArgs, opts ...pulumi.ResourceOption) (*GatewayComponent, error) {
	comp := &GatewayComponent{}
	err := ctx.RegisterComponentResource("CustomComponent:k8s:Gateway", name, comp, opts...)
	if err != nil {
		return nil, err
	}

	// Unmarshal listeners struct slice into dynamic map structure
	bytes, err := json.Marshal(args.Listeners)
	if err != nil {
		return nil, err
	}
	var listenersRaw []map[string]any
	if err := json.Unmarshal(bytes, &listenersRaw); err != nil {
		return nil, err
	}

	spec := pulumi.Map{
		"gatewayClassName": pulumi.String(args.ClassName),
		"listeners":        pulumi.ToMapArray(listenersRaw),
	}

	_, err = apiextensions.NewCustomResource(ctx, name+"-cr", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("gateway.networking.k8s.io/v1"),
		Kind:       pulumi.String("Gateway"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(args.Name),
			Namespace: pulumi.String(args.Namespace),
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
