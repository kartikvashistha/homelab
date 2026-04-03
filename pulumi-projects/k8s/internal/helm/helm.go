package helm

import (
	"fmt"

	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type HelmChart struct {
	Chart       string
	Namespace   string
	ReleaseName string
	Repo        string
	Values      pulumi.Map
	ValuesFile  string
	Version     string
}

func CreateHelmRelease(ctx *pulumi.Context, h HelmChart) (*helmv3.Release, error) {
	helmRelease, err := helmv3.NewRelease(ctx, h.ReleaseName, &helmv3.ReleaseArgs{
		Chart: pulumi.String(h.Chart),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String(h.Repo),
		},
		Version:         pulumi.String(h.Version),
		Name:            pulumi.String(h.ReleaseName),
		Namespace:       pulumi.String(h.Namespace),
		CreateNamespace: pulumi.Bool(true),
		Values:          pulumi.MapInput(h.Values),
		ValueYamlFiles:  pulumi.AssetOrArchiveArray{pulumi.NewFileAsset(h.ValuesFile)},
	})
	if err != nil {
		fmt.Printf("Error creating helm release: %v", h.ReleaseName)
		return nil, err
	}
	return helmRelease, nil
}
