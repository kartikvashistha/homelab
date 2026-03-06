package main

import (
	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	kustomizev2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize/v2"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
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

type Metallb struct {
	Install     bool
	AddressPool []string
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

		var metallb Metallb
		cfg.RequireObject("metallb", &metallb)
		if metallb.Install {
			metallbRelease, err := helmv3.NewRelease(ctx, "metallb", &helmv3.ReleaseArgs{
				Chart: pulumi.String("metallb"),
				RepositoryOpts: &helmv3.RepositoryOptsArgs{
					Repo: pulumi.String("https://metallb.github.io/metallb"),
				},
				Version:         pulumi.String("0.15.3"),
				Name:            pulumi.String("metallb"),
				Namespace:       pulumi.String("metallb-system"),
				CreateNamespace: pulumi.Bool(true),
			})
			if err != nil {
				return err
			}
			_, err = apiextensions.NewCustomResource(ctx, "metallbIpAddressPool", &apiextensions.CustomResourceArgs{
				ApiVersion: pulumi.String("metallb.io/v1beta1"),
				Kind:       pulumi.String("IPAddressPool"),
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String("first-pool"),
					Namespace: pulumi.String("metallb-system"),
				},
				OtherFields: kpulumi.UntypedArgs{
					"spec": pulumi.Map{
						"addresses": pulumi.ToStringArray(metallb.AddressPool),
					},
				},
			}, pulumi.DependsOn([]pulumi.Resource{metallbRelease}))

			if err != nil {
				return err
			}

			_, err = apiextensions.NewCustomResource(ctx, "metallbIpAddressPool", &apiextensions.CustomResourceArgs{
				ApiVersion: pulumi.String("metallb.io/v1beta1"),
				Kind:       pulumi.String("L2Advertisement"),
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String("advertisement"),
					Namespace: pulumi.String("metallb-system"),
				},
			}, pulumi.DependsOn([]pulumi.Resource{metallbRelease}))
			if err != nil {
				return err
			}
		}

		var installGatewayApiCrds bool
		cfg.RequireObject("installGatewayApiCrds", &installGatewayApiCrds)
		if installGatewayApiCrds {
			_, err := kustomizev2.NewDirectory(ctx, "gatewayapicrds", &kustomizev2.DirectoryArgs{
				Directory: pulumi.String("github.com/kubernetes-sigs/gateway-api/config/crd?ref=v1.4.0"),
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
