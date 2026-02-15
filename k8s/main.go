package main

import (
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
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

		metallb := HelmChart{
			ReleaseName: "metallb",
			Chart:       "metallb",
			Repo:        "https://metallb.github.io/metallb",
			Version:     "0.15.3",
			Namespace:   "metallb-system",
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

		coredns := HelmChart{
			ReleaseName: "coredns-external",
			Chart:       "coredns",
			Repo:        "https://coredns.github.io/helm",
			Version:     "1.45.2",
			Namespace:   "coredns",
			ValuesFile: pulumi.AssetOrArchiveArray{
				pulumi.NewFileAsset("./helm-values/coredns.yaml"),
			},
		}

		var HelmReleaseChartList []HelmChart
		HelmReleaseChartList = append(HelmReleaseChartList, cilium, coredns, metallb, traefik)

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
