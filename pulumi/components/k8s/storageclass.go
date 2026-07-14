package k8s

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	LONGHORN_RELEASE_NAME = "longhorn"
	LONGHORN_CHART        = "longhorn"
	LONGHORN_NAMESPACE    = "longhorn-system"
	LONGHORN_REPO         = "https://charts.longhorn.io"
	LONGHORN_VERSION      = "1.12.0"
)

type StorageClassComponent struct {
	pulumi.ResourceState
}
type StorageClassArgs struct {
}

func NewStorageClassComponent(ctx *pulumi.Context, name string, args *StorageClassArgs, opts ...pulumi.ResourceOption) (*StorageClassComponent, error) {
	comp := &StorageClassComponent{}
	if err := ctx.RegisterComponentResource("CustomComponent:k8s:StorageClass", name, comp, opts...); err != nil {
		return nil, err
	}

	longhornNs, err := corev1.NewNamespace(ctx, LONGHORN_NAMESPACE+"-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(LONGHORN_NAMESPACE),
		},
	}, pulumi.Parent(comp))

	if err != nil {
		return nil, fmt.Errorf("Error encountered during the creation of longhorn's namespace: %w", err)
	}

	_, err = helmv3.NewRelease(ctx, LONGHORN_RELEASE_NAME, &helmv3.ReleaseArgs{
		Chart:     pulumi.String(LONGHORN_CHART),
		Namespace: pulumi.String(LONGHORN_NAMESPACE),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String(LONGHORN_REPO),
		},
		Name:    pulumi.String(LONGHORN_RELEASE_NAME),
		Version: pulumi.String(LONGHORN_VERSION),
	},
		pulumi.Parent(comp),
		pulumi.DependsOn([]pulumi.Resource{longhornNs}))

	if err != nil {
		return nil, fmt.Errorf("Error encountered during the creation of longhorn's Helm Release: %w", err)
	}
	return comp, nil
}
