package core

import (
	"fmt"

	kustomizev2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

type NetworkComponentArgs struct {
	InstallGatewayApiCrds bool        `json:"installGatewayApiCrds"`
	Metallb               MetalLBArgs `json:"metallb"`
}

// NetworkComponent groups cluster networking: Gateway API CRDs, MetalLB
// (optional) and Istio (ambient mode).
type NetworkComponent struct {
	pulumi.ResourceState
}

func NewNetworkComponent(
	ctx *pulumi.Context,
	name string,
	args *NetworkComponentArgs,
	opts ...pulumi.ResourceOption,
) (*NetworkComponent, error) {
	comp := &NetworkComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:Networking", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register networking component: %w", err)
	}

	if args.InstallGatewayApiCrds {
		if _, err := kustomizev2.NewDirectory(ctx, "gateway-api-crds", &kustomizev2.DirectoryArgs{
			Directory: pulumi.String(fmt.Sprintf(
				"github.com/kubernetes-sigs/gateway-api/config/crd?ref=%s",
				versions.GatewayAPI,
			)),
		}, pulumi.Parent(comp)); err != nil {
			return nil, fmt.Errorf("install gateway api crds: %w", err)
		}
	}

	if err := newMetalLB(ctx, &args.Metallb, comp); err != nil {
		return nil, err
	}

	if err := newIstio(ctx, comp); err != nil {
		return nil, err
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{}); err != nil {
		return nil, err
	}
	return comp, nil
}
