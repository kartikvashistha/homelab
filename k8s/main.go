package main

import (
	// appsv1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apps/v1"
	// corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	// helmv4 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v4"
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
	Values      pulumi.Map
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {

		cilium := HelmChart{
			ReleaseName: "cilium",
			Chart:       "cilium",
			Repo:        "https://helm.cilium.io",
			Version:     "1.18.5",
			Namespace:   "kube-system",
			ValuesFile: pulumi.AssetOrArchiveArray{
				pulumi.NewFileAsset("./helm-values/cilium.yaml"),
			},
		}

		traefik := HelmChart{
			ReleaseName: "traefik",
			Chart:       "traefik",
			Repo:        "https://traefik.github.io/charts",
			Version:     "38.0.1",
			Namespace:   "traefik",
			ValuesFile: pulumi.AssetOrArchiveArray{
				pulumi.NewFileAsset("./helm-values/traefik.yaml"),
			},
		}

		var HelmReleaseChartList []HelmChart
		HelmReleaseChartList = append(HelmReleaseChartList, cilium, traefik)

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
				ValueYamlFiles:  v.ValuesFile,
			})

			if err != nil {
				return err
			}
		}
		return nil
	})
}
