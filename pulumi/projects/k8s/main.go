package main

import (
	// "fmt"
	// "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/kartikvashistha/homelab/pulumi/components/k8s"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")

		// kubectx := config.New(ctx, "kubernetes").Require("context")

		var n k8s.NetworkComponentArgs
		cfg.RequireObject("networking", &n)
		_, err := k8s.SetupNetworkingComponents(ctx, "core-networking", &n)

		if err != nil {
			return err
		}

		// var core k8sCore
		// cfg.RequireObject("core", &core)
		// err := bootstrapCoreServices(ctx, core)
		// if err != nil {
		// 	fmt.Printf("Error during the installation of Core packages!")
		// 	return err
		// }
		//
		// err = bootstrapCertManager(ctx, kubectx)
		// if err != nil {
		// 	fmt.Println("Error in core service setup: cert-manager")
		// 	return err
		// }
		//
		// var helmChartList []helm.HelmChart
		// cfg.RequireObject("helm", &helmChartList)
		// for _, v := range helmChartList {
		// 	file := fmt.Sprintf("./helm-overrides/%s/%s/values.yaml", kubectx, v.ReleaseName)
		// 	_, err := os.Stat(file)
		//
		// 	if err == nil {
		// 		v.ValuesFile = file
		// 	}
		//
		// 	_, err = helm.CreateHelmRelease(ctx, v)
		// 	if err != nil {
		// 		fmt.Println("Error during the creation of helm release!")
		// 		return err
		// 	}
		// }
		//
		// err = bootstrapDnsResolver(ctx, kubectx)
		// if err != nil {
		// 	fmt.Println("Error during installation of coredns-external resolver!")
		// 	return err
		// }
		//
		// _, err = yaml.NewConfigGroup(ctx, "manifests",
		// 	&yaml.ConfigGroupArgs{
		// 		Files: []string{fmt.Sprintf("./manifests/%s/*.yaml", kubectx), fmt.Sprintf("./manifests/%s/*.yml", kubectx)},
		// 	},
		// )
		// if err != nil {
		// 	return err
		// }

		return nil
	})
}
