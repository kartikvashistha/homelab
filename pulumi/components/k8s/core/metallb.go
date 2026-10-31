package core

import (
	"errors"
	"fmt"

	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

const (
	metalLBNamespace = "metallb-system"
	metalLBPoolName  = "homelab-pool"
)

type MetalLBArgs struct {
	Install     bool     `json:"install"`
	AddressPool []string `json:"addressPool"`
}

// newMetalLB installs MetalLB in L2 mode: namespace -> chart -> address pool ->
// L2 advertisement. It is a no-op unless args.Install is set.
func newMetalLB(ctx *pulumi.Context, args *MetalLBArgs, parent pulumi.Resource) error {
	if !args.Install {
		return nil
	}
	if len(args.AddressPool) == 0 {
		return errors.New("metallb: addressPool must not be empty when install is true")
	}

	parentOpt := pulumi.Parent(parent)

	ns, err := corev1.NewNamespace(ctx, "metallb-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(metalLBNamespace)},
	}, parentOpt)
	if err != nil {
		return fmt.Errorf("create metallb namespace: %w", err)
	}

	release, err := helmv3.NewRelease(ctx, "metallb", &helmv3.ReleaseArgs{
		Name:           pulumi.String("metallb"),
		Chart:          pulumi.String("metallb"),
		Namespace:      ns.Metadata.Name(),
		Lint:           pulumi.Bool(true),
		Version:        pulumi.String(versions.MetalLB),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(versions.MetalLBRepo)},
	}, parentOpt)
	if err != nil {
		return fmt.Errorf("install metallb: %w", err)
	}

	pool, err := apiextensions.NewCustomResource(ctx, "metallb-ip-pool", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("metallb.io/v1beta1"),
		Kind:       pulumi.String("IPAddressPool"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String(metalLBPoolName),
			Namespace: ns.Metadata.Name(),
		},
		OtherFields: kpulumi.UntypedArgs{
			"spec": pulumi.Map{
				"addresses": pulumi.ToStringArray(args.AddressPool),
			},
		},
	}, parentOpt, pulumi.DependsOn([]pulumi.Resource{release}))
	if err != nil {
		return fmt.Errorf("create metallb address pool: %w", err)
	}

	if _, err := apiextensions.NewCustomResource(ctx, "metallb-l2-advertisement", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("metallb.io/v1beta1"),
		Kind:       pulumi.String("L2Advertisement"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("homelab-l2"),
			Namespace: ns.Metadata.Name(),
		},
		OtherFields: kpulumi.UntypedArgs{
			"spec": pulumi.Map{
				"ipAddressPools": pulumi.ToStringArray([]string{metalLBPoolName}),
			},
		},
	}, parentOpt, pulumi.DependsOn([]pulumi.Resource{pool})); err != nil {
		return fmt.Errorf("create metallb l2 advertisement: %w", err)
	}

	return nil
}
