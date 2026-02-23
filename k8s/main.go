package main

import (
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type HelmChart struct {
	Chart       string
	Namespace   string
	ReleaseName string
	Repo        string
	Version     string
	ValuesFile  string
	Values      pulumi.Map
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		var cni bool

		cfg.RequireObject("installCni", &cni)
		if cni {
			_, err := helmv3.NewRelease(ctx, "cilium", &helmv3.ReleaseArgs{
				Chart: pulumi.String("cilium"),
				RepositoryOpts: &helmv3.RepositoryOptsArgs{
					Repo: pulumi.String("https://helm.cilium.io"),
				},
				Version:         pulumi.String("1.18.5"),
				Name:            pulumi.String("cilium"),
				Namespace:       pulumi.String("kube-system"),
				CreateNamespace: pulumi.Bool(true),
				ValueYamlFiles:  pulumi.AssetOrArchiveArray{pulumi.NewFileAsset("./helm-values/cilium.yaml")},
			})
			if err != nil {
				return err
			}
		}

		var HelmReleaseChartList []HelmChart
		cfg.RequireObject("helmCharts", &HelmReleaseChartList)

		for _, v := range HelmReleaseChartList {
			_, err := helmv3.NewRelease(ctx, v.ReleaseName, &helmv3.ReleaseArgs{
				Chart: pulumi.String(v.Chart),
				RepositoryOpts: &helmv3.RepositoryOptsArgs{
					Repo: pulumi.String(v.Repo),
				},
				Version:         pulumi.String(v.Version),
				Name:            pulumi.String(v.ReleaseName),
				Namespace:       pulumi.String(v.Namespace),
				CreateNamespace: pulumi.Bool(true),
				ValueYamlFiles:  pulumi.AssetOrArchiveArray{pulumi.NewFileAsset(v.ValuesFile)},
			})
			if err != nil {
				return err
			}
		}
		return nil
	})
}
