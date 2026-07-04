package main

import (
	"fmt"

	"k8s-flux/internal/helm"
	"os"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	kustomizev2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize/v2"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"gopkg.in/yaml.v3"
)

const (
	GATEWAYAPI_CRDS_VERSION = "v1.4.0"
)

type Metallb struct {
	Install     bool
	AddressPool []string
}

type Core struct {
	InstallGatewayApiCrds bool
	Metallb               Metallb
	HelmCharts            []helm.HelmChart
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		kubectx := config.New(ctx, "kubernetes").Require("context")

		var core Core
		cfg.RequireObject("core", &core)

		err := setupFluxOperator(ctx)
		if err != nil {
			fmt.Println("Error during the setup of flux operator!")
			return err
		}

		if core.InstallGatewayApiCrds {
			_, err := kustomizev2.NewDirectory(ctx, "gatewayapicrds", &kustomizev2.DirectoryArgs{
				Directory: pulumi.String(fmt.Sprintf("github.com/kubernetes-sigs/gateway-api/config/crd?ref=%s", GATEWAYAPI_CRDS_VERSION))})

			if err != nil {
				fmt.Println("Error during the installation of Gateway API CRDs!")
				return err
			}
		}

		if core.Metallb.Install {
			err := setupMetallb(ctx, core.Metallb)
			if err != nil {
				fmt.Println("Error during the setup of metallb!")
				return err
			}
		}
		err = setupCoreAddons(ctx, &core.HelmCharts, &kubectx)
		if err != nil {
			return err
		}

		return nil
	})
}

func setupCoreAddons(ctx *pulumi.Context, helmCharts *[]helm.HelmChart, kubectx *string) error {
	var inputsList pulumi.MapArray
	for _, item := range *helmCharts {
		var helmValuesOverrides any
		filePath := fmt.Sprintf("./helm-overrides/%s/%s/values.yaml", *kubectx, item.ReleaseName)

		if data, err := os.ReadFile(filePath); err == nil {
			_ = yaml.Unmarshal(data, &helmValuesOverrides)
		} else {
			helmValuesOverrides = map[string]any{}
		}

		inputsList = append(inputsList, pulumi.Map{
			"repoName":     pulumi.String(item.Chart),
			"repoUrl":      pulumi.String(item.Repo),
			"chartName":    pulumi.String(item.Chart),
			"chartVersion": pulumi.String(item.Version),
			"releaseName":  pulumi.String(item.ReleaseName),
			"namespace":    pulumi.String(item.Namespace),
			"values":       pulumi.Any(helmValuesOverrides),
		})
	}

	resourcesTemplates := pulumi.MapArray{
		pulumi.Map{
			"apiVersion": pulumi.String("v1"),
			"kind":       pulumi.String("Namespace"),
			"metadata": pulumi.Map{
				"name": pulumi.String("<< inputs.namespace >>"),
			},
		},
		pulumi.Map{
			"apiVersion": pulumi.String("source.toolkit.fluxcd.io/v1"),
			"kind":       pulumi.String("HelmRepository"),
			"metadata": pulumi.Map{
				"name":      pulumi.String("<< inputs.repoName >>"),
				"namespace": pulumi.String("<< inputs.namespace >>"),
			},
			"spec": pulumi.Map{
				"interval": pulumi.String("1m"),
				"url":      pulumi.String("<< inputs.repoUrl >>"),
			},
		},
		pulumi.Map{
			"apiVersion": pulumi.String("helm.toolkit.fluxcd.io/v2"),
			"kind":       pulumi.String("HelmRelease"),
			"metadata": pulumi.Map{
				"name":      pulumi.String("<< inputs.releaseName >>"),
				"namespace": pulumi.String("<< inputs.namespace >>"),
			},
			"spec": pulumi.Map{
				"interval": pulumi.String("1m"),
				"install": pulumi.Map{
					"remediation": pulumi.Map{
						"retries": pulumi.Int(-1),
					},
				},
				"chart": pulumi.Map{
					"spec": pulumi.Map{
						"chart":   pulumi.String("<< inputs.chartName >>"),
						"version": pulumi.String("<< inputs.chartVersion >>"),
						"sourceRef": pulumi.Map{
							"kind":      pulumi.String("HelmRepository"),
							"name":      pulumi.String("<< inputs.repoName >>"),
							"namespace": pulumi.String("<< inputs.namespace >>"),
						},
					},
				},
				"values": pulumi.String("<< inputs.values | toJson >>"),
			},
		},
	}

	_, err := apiextensions.NewCustomResource(ctx, "coreaddons-resourceset",
		&apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("fluxcd.controlplane.io/v1"),
			Kind:       pulumi.String("ResourceSet"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("core-addons-generator"),
				Namespace: pulumi.String("default"),
				Annotations: pulumi.StringMap{
					"fluxcd.controlplane.io/reconcile":      pulumi.String("enabled"),
					"fluxcd.controlplane.io/reconcileEvery": pulumi.String("10m"),
				},
			},
			OtherFields: map[string]any{
				"spec": pulumi.Map{
					"inputs":    pulumi.Any(inputsList),
					"resources": pulumi.Any(resourcesTemplates),
				},
			},
		},
	)
	if err != nil {
		return fmt.Errorf("Failed to create dynamic ResourceSet for coreaddons: %w", err)
	}
	return nil
}
