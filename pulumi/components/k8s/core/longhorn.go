package core

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

const longhornNamespace = "longhorn-system"

type StorageArgs struct {
	Longhorn LonghornArgs `json:"longhorn"`
}

type LonghornArgs struct {
	Install bool `json:"install"`
}

// LonghornComponent installs Longhorn, which provides the cluster's
// replicated block storage and default StorageClass.
type LonghornComponent struct {
	pulumi.ResourceState
}

func NewLonghornComponent(
	ctx *pulumi.Context,
	name string,
	opts ...pulumi.ResourceOption,
) (*LonghornComponent, error) {
	comp := &LonghornComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:Longhorn", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register longhorn component: %w", err)
	}
	parentOpt := pulumi.Parent(comp)

	ns, err := corev1.NewNamespace(ctx, "longhorn-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(longhornNamespace)},
	}, parentOpt)
	if err != nil {
		return nil, fmt.Errorf("create longhorn namespace: %w", err)
	}

	if _, err := helmv3.NewRelease(ctx, "longhorn", &helmv3.ReleaseArgs{
		Name:           pulumi.String("longhorn"),
		Chart:          pulumi.String("longhorn"),
		Namespace:      ns.Metadata.Name(),
		Version:        pulumi.String(versions.Longhorn),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(versions.LonghornRepo)},
	}, parentOpt); err != nil {
		return nil, fmt.Errorf("install longhorn: %w", err)
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{}); err != nil {
		return nil, err
	}
	return comp, nil
}
