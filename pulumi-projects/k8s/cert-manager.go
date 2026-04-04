package main

import (
	"fmt"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"k8s/internal/helm"
)

const (
	CERT_MANAGER_RELEASE_NAME  = "cert-manager"
	CERT_MANAGER_CHART_NAME    = "cert-manager"
	CERT_MANAGER_NAMESPACE     = "cert-manager"
	CERT_MANAGER_CHART_REPO    = "https://charts.jetstack.io"
	CERT_MANAGER_CHART_VERSION = "1.20.0"
)

func bootstrapCertManager(ctx *pulumi.Context, k string) error {
	certmanagerHelmRelease, err := helm.CreateHelmRelease(ctx, helm.HelmChart{
		Chart:       CERT_MANAGER_CHART_NAME,
		Repo:        CERT_MANAGER_CHART_REPO,
		Version:     CERT_MANAGER_CHART_VERSION,
		ReleaseName: CERT_MANAGER_RELEASE_NAME,
		Namespace:   CERT_MANAGER_NAMESPACE,
		Values: pulumi.Map{
			"crds": pulumi.Map{
				"enabled": pulumi.Bool(true),
			},
			"config": pulumi.Map{
				"enableGatewayAPI": pulumi.Bool(true),
			},
		},
	})
	if err != nil {
		fmt.Println("Error creating cert manager helm release!")
		return err
	}

	// Setup a selfsigned issuer, ca and cluster issuer
	_, err = yaml.NewConfigGroup(ctx, "cert-manager-self-signed-issuer-setup", &yaml.ConfigGroupArgs{
		Files: []string{fmt.Sprintf("./config/%s/self-signed-ca-setup.yaml", k)},
	}, pulumi.DependsOn([]pulumi.Resource{certmanagerHelmRelease}),
	)
	if err != nil {
		fmt.Println("Error during the setup of the self signed issuer!")
		return err
	}

	return nil
}
