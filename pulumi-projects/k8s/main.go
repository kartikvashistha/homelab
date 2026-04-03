package main

import (
	"fmt"
	"k8s/internal/helm"
	"k8s/internal/metallb"
	"os"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type k8sCore struct {
	InstallGatewayApiCrds bool
	Metallb               metallb.Metallb
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		var kubectx string
		if ctx.Stack() == "dreadnought" {
			kubectx = config.New(ctx, "kubernetes").Require("context")
		} else {
			kubectx = "dreadnought"
		}

		var core k8sCore
		cfg.RequireObject("core", &core)
		err := BootstrapCoreServices(ctx, core)
		if err != nil {
			fmt.Printf("Error during the installation of Core packages!")
			return err
		}

		var helmChartList []helm.HelmChart
		cfg.RequireObject("helm", &helmChartList)
		for _, v := range helmChartList {
			file := fmt.Sprintf("./helm-overrides/%s/%s/values.yaml", kubectx, v.ReleaseName)
			_, err := os.Stat(file)

			if err == nil {
				v.ValuesFile = file
			}

			_, err = helm.CreateHelmRelease(ctx, v)
			if err != nil {
				fmt.Println("Error during the creation of helm release!")
				return err
			}
		}

		_, err = yaml.NewConfigGroup(ctx, "manifests",
			&yaml.ConfigGroupArgs{
				Files: []string{fmt.Sprintf("./manifests/%s/*.yaml", kubectx), fmt.Sprintf("./manifests/%s/*.yml", kubectx)},
			},
		)
		if err != nil {
			return err
		}

		return nil
	})
}
