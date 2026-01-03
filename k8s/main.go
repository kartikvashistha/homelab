package main

import (
	// appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	// corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
	// metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

type HelmChart struct {
	Chart       string
	Namespace   string
	ReleaseName string
	Repo        string
	Version     string
	ValuesFile  pulumi.AssetOrArchiveArray
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		var ChartList []HelmChart

		cilium := HelmChart{
			ReleaseName: "cilium",
			Chart:       "cilium",
			Repo:        "https://helm.cilium.io",
			Version:     "1.18.5",
			Namespace:   "kube-system",
		}

		ChartList = append(ChartList, cilium)

		for _, v := range ChartList {
			_, err := helmv4.NewChart(ctx, v.ReleaseName, &helmv4.ChartArgs{
				Chart: pulumi.String(v.Chart),
				RepositoryOpts: &helmv4.RepositoryOptsArgs{
					Repo: pulumi.String(v.Repo),
				},
				Version:   pulumi.String(v.Version),
				Name:      pulumi.String(v.ReleaseName),
				Namespace: pulumi.String(v.Namespace),
			})

			if err != nil {
				return err
			}
		}
		return nil
	})
}
