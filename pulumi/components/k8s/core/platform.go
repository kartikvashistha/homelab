package core

import (
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// PlatformArgs is the cluster platform's configuration, one field per
// stack-config key (`k8s:networking`, `k8s:storage`, `k8s:gateway`).
type PlatformArgs struct {
	Networking NetworkComponentArgs `json:"networking"`
	Storage    StorageArgs          `json:"storage"`
	Gateway    GatewayArgs          `json:"gateway"`
}

// Platform is everything a cluster needs before workloads can land:
//
//	Networking (CRDs, MetalLB, Istio) -> CertManager -> Gateway
//	Longhorn (optional, independent)
//
// Istio, cert-manager and the gateway are always installed because the
// gateway can't exist without them; MetalLB and Longhorn are opt-in.
type Platform struct {
	pulumi.ResourceState

	Networking  *NetworkComponent
	CertManager *CertManagerComponent
	Storage     *LonghornComponent // nil unless storage.longhorn.install
	Gateway     *GatewayComponent
}

func NewPlatform(
	ctx *pulumi.Context,
	name string,
	args *PlatformArgs,
	opts ...pulumi.ResourceOption,
) (*Platform, error) {
	if err := args.Gateway.Validate(); err != nil {
		return nil, fmt.Errorf("gateway config: %w", err)
	}

	p := &Platform{}
	if err := ctx.RegisterComponentResource("homelab:k8s:Platform", name, p, opts...); err != nil {
		return nil, fmt.Errorf("register platform component: %w", err)
	}
	parent := pulumi.Parent(p)

	var err error

	p.Networking, err = NewNetworkComponent(ctx, "networking", &args.Networking, parent)
	if err != nil {
		return nil, fmt.Errorf("networking: %w", err)
	}

	p.CertManager, err = NewCertManagerComponent(ctx, "cert-manager", &CertManagerArgs{
		InstallCrds:      true,
		EnableGatewayAPI: args.Networking.InstallGatewayApiCrds,
		SelfSignedCA:     true,
	}, parent, pulumi.DependsOn([]pulumi.Resource{p.Networking}))
	if err != nil {
		return nil, fmt.Errorf("cert-manager: %w", err)
	}

	gateway := args.Gateway // copy: ClusterIssuer is derived, not configured
	gateway.ClusterIssuer = p.CertManager.ClusterIssuer
	p.Gateway, err = NewGatewayComponent(ctx, "gateway", &gateway, parent,
		pulumi.DependsOn([]pulumi.Resource{p.Networking, p.CertManager}))
	if err != nil {
		return nil, fmt.Errorf("gateway: %w", err)
	}

	if args.Storage.Longhorn.Install {
		args.Storage.Longhorn.Gateway = p.Gateway
		p.Storage, err = NewLonghornComponent(ctx, "longhorn", args.Storage.Longhorn, parent)
		if err != nil {
			return nil, fmt.Errorf("storage: %w", err)
		}
	}

	if err := ctx.RegisterResourceOutputs(p, pulumi.Map{}); err != nil {
		return nil, err
	}
	return p, nil
}

// Infra is a dependency option for workloads that need the cluster's
// networking and certificate machinery to be ready first.
func (p *Platform) Infra() pulumi.ResourceOption {
	return pulumi.DependsOn([]pulumi.Resource{p.Networking, p.CertManager})
}
