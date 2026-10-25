package core

import (
	"fmt"

	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

const CiliumNamespace = "kube-system"

func newCilium(ctx *pulumi.Context, parent pulumi.Resource) error {

	_, err := helmv3.NewRelease(ctx, "cilium-cni", &helmv3.ReleaseArgs{
		Name:           pulumi.String("cilium"),
		Namespace:      pulumi.String("kube-system"),
		Chart:          pulumi.String("cilium"),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(versions.CiliumRepo)},
		Version:        pulumi.String(versions.Cilium),
		WaitForJobs:    pulumi.Bool(true),
		Lint:           pulumi.Bool(true),
		Values: pulumi.Map{
			"ipam": pulumi.Map{
				"operator": pulumi.Map{
					"clusterPoolIPv4PodCIDRList": pulumi.ToStringArray([]string{"10.0.0.0/8"}),
				},
			},
			"cni": pulumi.Map{
				"exclusive": pulumi.Bool(false),
			},
		},
	}, pulumi.Parent(parent))

	if err != nil {
		return fmt.Errorf("install %s: %w", "cilium", err)
	}

	return nil
}
