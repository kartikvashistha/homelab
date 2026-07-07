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
	"k8s-flux/components/flux"
)

const (
	GATEWAYAPI_CRDS_VERSION = "v1.4.0"

	COREDNS_RELEASE_NAME  = "coredns-external"
	COREDNS_CHART_NAME    = "coredns"
	COREDNS_NAMESPACE     = "coredns"
	COREDNS_CHART_REPO    = "https://coredns.github.io/helm"
	COREDNS_CHART_VERSION = "1.45.2"
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

type DnsServer struct {
	Enabled   bool
	ServiceIp string
	Mappings  struct {
		Lbip  string
		Fqdns []string
	}
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		var helmConfig []helm.HelmChart
		var core Core
		var dnsServer DnsServer

		cfg := config.New(ctx, "")
		kubectx := config.New(ctx, "kubernetes").Require("context")

		cfg.RequireObject("core", &core)
		cfg.RequireObject("dns", &dnsServer)

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
			helmConfig = append(helmConfig, helm.HelmChart{
				Chart:       METALLB_CHART_NAME,
				Repo:        METALLB_CHART_REPO,
				Version:     METALLB_CHART_VERSION,
				ReleaseName: METALLB_RELEASE_NAME,
				Namespace:   METALLB_NAMESPACE,
			})
		}

		if dnsServer.Enabled {
			helmConfig = append(helmConfig, helm.HelmChart{
				Chart:       COREDNS_CHART_NAME,
				Repo:        COREDNS_CHART_REPO,
				Version:     COREDNS_CHART_VERSION,
				ReleaseName: COREDNS_RELEASE_NAME,
				Namespace:   COREDNS_NAMESPACE,
				Values: pulumi.Map{
					"serviceType": pulumi.String("LoadBalancer"),
					"service": pulumi.Map{
						"loadBalancerIP": pulumi.String(dnsServer.ServiceIp),
					},
					"replicaCount": pulumi.String("2"),
				},
			})
		}

		helmConfig = append(helmConfig, core.HelmCharts...)
		err = helmReleaseGenerator(ctx, &helmConfig, &kubectx)
		if err != nil {
			return err
		}

		_, err = flux.HelmResourceSet(ctx, "test-flux-component", &flux.ResourceSetArgs{
			ChartName:        pulumi.String("headlamp"),
			ChartVersion:     pulumi.String("0.41.0"),
			ReleaseName:      pulumi.String("headlamp"),
			ReleaseNamespace: pulumi.String("headlamp"),
			RepoName:         pulumi.String("headlamp"),
			RepoUrl:          pulumi.String("https://kubernetes-sigs.github.io/headlamp"),
			Values:           pulumi.Map{},
		})

		if err != nil {
			return fmt.Errorf("Error creating our custom FluxComponent Resource: %w", err)
		}

		err = setupMetallb(ctx, core.Metallb)
		if err != nil {
			fmt.Println("Error during the setup of metallb!")
			return err
		}
		return nil
	})
}

func helmReleaseGenerator(ctx *pulumi.Context, helmCharts *[]helm.HelmChart, kubectx *string) error {
	var inputsList pulumi.MapArray
	for _, item := range *helmCharts {
		var helmValuesOverrides any
		filePath := fmt.Sprintf("./manifests/%s/helm-overrides/%s/values.yaml", *kubectx, item.ReleaseName)

		if data, err := os.ReadFile(filePath); err == nil {
			_ = yaml.Unmarshal(data, &helmValuesOverrides)
		} else {
			helmValuesOverrides = map[string]any{}
		}

		inputsList = append(inputsList, pulumi.Map{
			"chartName":    pulumi.String(item.Chart),
			"chartVersion": pulumi.String(item.Version),
			"namespace":    pulumi.String(item.Namespace),
			"releaseName":  pulumi.String(item.ReleaseName),
			"repoName":     pulumi.String(item.Chart),
			"repoUrl":      pulumi.String(item.Repo),
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

	_, err := apiextensions.NewCustomResource(ctx, "helmReleaseResourceSet",
		&apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("fluxcd.controlplane.io/v1"),
			Kind:       pulumi.String("ResourceSet"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("helm-release-generator"),
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
