package main

import (
	"fmt"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	kustomizev2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize/v2"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"k8s/internal/certmanager"
	"k8s/internal/helm"
	"k8s/internal/metallb"
	"os"
)

const (
	COREDNS_RELEASE_NAME  = "coredns-external"
	COREDNS_CHART_NAME    = "coredns"
	COREDNS_NAMESPACE     = "coredns"
	COREDNS_CHART_REPO    = "https://coredns.github.io/helm"
	COREDNS_CHART_VERSION = "1.45.2"

	GATEWAYAPI_CRDS_VERSION = "v1.4.0"
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
		err := bootstrapCoreServices(ctx, core)
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

		err = bootstrapDnsResolver(ctx, kubectx)
		if err != nil {
			fmt.Println("Error during installation of coredns-external resolver!")
			return err
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

func bootstrapCoreServices(ctx *pulumi.Context, k k8sCore) error {
	if k.Metallb.Install {
		err := metallb.BootstrapMetallb(ctx, k.Metallb)
		if err != nil {
			fmt.Printf("Error during bootstrapping of Metallb: %v", err)
			return err
		}
	}

	if k.InstallGatewayApiCrds {
		_, err := kustomizev2.NewDirectory(ctx, "gatewayapicrds", &kustomizev2.DirectoryArgs{
			Directory: pulumi.String(fmt.Sprintf("github.com/kubernetes-sigs/gateway-api/config/crd?ref=%s", GATEWAYAPI_CRDS_VERSION))})

		if err != nil {
			fmt.Println("Error during the installation of Gateway API CRDs!")
			return err
		}
	}

	err := certmanager.BootstrapCertManager(ctx)
	if err != nil {
		fmt.Println("Error in core service setup: cert-manager")
		return err
	}

	return nil
}

func bootstrapDnsResolver(ctx *pulumi.Context, k string) error {
	corednsNS, err := corev1.NewNamespace(ctx, "coredns-external-namespace", &corev1.NamespaceArgs{
		ApiVersion: pulumi.String("string"),
		Kind:       pulumi.String("string"),
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(COREDNS_NAMESPACE)},
	})
	if err != nil {
		fmt.Printf("Error during the creation of namespace:%v", COREDNS_NAMESPACE)
		return err
	}

	_, err = yaml.NewConfigGroup(ctx, "coredns-external-configmap",
		&yaml.ConfigGroupArgs{
			Files: []string{fmt.Sprintf("./config/%s/cfg-private-dns-hosts.yaml", k)},
		}, pulumi.DependsOn([]pulumi.Resource{corednsNS}))
	if err != nil {
		fmt.Println("Error occured during creation of configmap: private-dns-hosts")
		return err
	}

	_, err = helm.CreateHelmRelease(ctx, helm.HelmChart{
		Chart:       COREDNS_CHART_NAME,
		Repo:        COREDNS_CHART_REPO,
		Version:     COREDNS_CHART_VERSION,
		ReleaseName: COREDNS_RELEASE_NAME,
		Namespace:   COREDNS_NAMESPACE,
		ValuesFile:  fmt.Sprintf("./helm-overrides/%s/coredns-external/values.yaml", k),
	})

	if err != nil {
		fmt.Println("Error encountered during coredns installalation!")
		return err
	}
	return nil
}
