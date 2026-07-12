package main

import (
	"fmt"
	"os"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type HelmConfig struct {
	ReleaseName string
	Chart       string
	Namespace   string
	Repo        string
	Version     string
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		kubectx := config.New(ctx, "kubernetes").Require("context")

		var nc k8s.NetworkComponentArgs
		cfg.RequireObject("networking", &nc)
		networkComponent, err := k8s.SetupNetworkingComponents(ctx, "core-networking", &nc)
		if err != nil {
			return err
		}

		certManager, err := k8s.SetupCertManagerComponents(ctx, "cert-manager", &k8s.CertManagerArgs{
			EnableGatewayAPI: nc.InstallGatewayApiCrds,
			InstallCrds:      true,
		})
		if err != nil {
			return err
		}

		var h []HelmConfig
		cfg.RequireObject("helm", &h)
		for _, v := range h {
			var file string
			file = fmt.Sprintf("./clusters/%s/helm-overrides/%s/values.yaml", kubectx, v.ReleaseName)
			_, err := os.Stat(file)

			_, err = helmv3.NewRelease(ctx, v.ReleaseName, &helmv3.ReleaseArgs{
				Chart:     pulumi.String(v.Chart),
				Namespace: pulumi.String(v.Namespace),
				RepositoryOpts: &helmv3.RepositoryOptsArgs{
					Repo: pulumi.String(v.Repo),
				},
				Name:            pulumi.String(v.ReleaseName),
				Version:         pulumi.String(v.Version),
				CreateNamespace: pulumi.Bool(true),
				ValueYamlFiles:  pulumi.AssetOrArchiveArray{pulumi.NewFileAsset(file)},
			}, pulumi.DependsOn([]pulumi.Resource{networkComponent, certManager}))
			if err != nil {
				return fmt.Errorf("error during the creation of helm release: %v", v.ReleaseName)
			}
		}

		_, err = yaml.NewConfigGroup(ctx, "manifests",
			&yaml.ConfigGroupArgs{
				Files: []string{
					fmt.Sprintf("./clusters/%s/manifests/*.yaml", kubectx),
					fmt.Sprintf("./clusters/%s/manifests/*.yml", kubectx),
				},
			}, pulumi.DependsOn([]pulumi.Resource{networkComponent, certManager}),
		)
		if err != nil {
			return fmt.Errorf("error during the creation of yaml config group: %v", err)
		}
		return nil
	})
}
