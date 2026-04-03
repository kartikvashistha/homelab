package main

import (
	"fmt"
	"k8s/internal/certmanager"
	"k8s/internal/metallb"

	kustomizev2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	GATEWAYAPI_CRDS_VERSION = "v1.4.0"
)

func BootstrapCoreServices(ctx *pulumi.Context, k k8sCore) error {
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
