package main

import (
	"fmt"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	// "github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
	"k8s-flux/internal/helm"
)

// HelmInput represents the structural matrix data passed directly to the CRD
type HelmInput struct {
	RepoName     string
	RepoURL      string
	ChartName    string
	ChartVersion string
	ReleaseName  string
	Namespace    string
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		// cfg := config.New(ctx, "")
		// kubectx := config.New(ctx, "kubernetes").Require("context")

		//TODO: Bootstrap flux operator
		coreAddonsHelm := []helm.HelmChart{
			{
				Chart:       "metallb",
				Namespace:   "metallb-system",
				ReleaseName: "metallb",
				Repo:        "https://metallb.github.io/metallb",
				Version:     "0.15.3",
			},
		}

		// 1. Your clean list of items
		// chartData := []HelmInput{
		// 	{
		// 		RepoName:     "bitnami",
		// 		RepoURL:      "https://charts.bitnami.com/bitnami",
		// 		ChartName:    "redis",
		// 		ChartVersion: "18.0.0",
		// 		ReleaseName:  "redis-cache",
		// 		Namespace:    "default",
		// 	},
		// 	{
		// 		RepoName:     "ingress-nginx",
		// 		RepoURL:      "https://kubernetes.github.io/ingress-nginx",
		// 		ChartName:    "ingress-nginx",
		// 		ChartVersion: "4.10.0",
		// 		ReleaseName:  "nginx-gateway",
		// 		Namespace:    "default",
		// 	},
		// }

		// 2. Map the Go slice straight into a Pulumi MapArray for the "inputs" block
		var inputsList pulumi.MapArray
		for _, item := range coreAddonsHelm {
			inputsList = append(inputsList, pulumi.Map{
				"repoName":     pulumi.String(item.Chart),
				"repoUrl":      pulumi.String(item.Repo),
				"chartName":    pulumi.String(item.ReleaseName),
				"chartVersion": pulumi.String(item.Version),
				"releaseName":  pulumi.String(item.ReleaseName),
				"namespace":    pulumi.String(item.Namespace),
			})
		}

		// 3. Define the inline templates that the Flux Operator will evaluate
		// using its custom delimiters `<< inputs.fieldName >>` under the `resources` field.
		resourcesTemplates := pulumi.MapArray{
			pulumi.Map{
				"apiVersion": pulumi.String("source.toolkit.fluxcd.io/v1"),
				"kind":       pulumi.String("HelmRepository"),
				"metadata": pulumi.Map{
					"name":      pulumi.String("<< inputs.repoName >>"),
					"namespace": pulumi.String("<< inputs.namespace >>"),
				},
				"spec": pulumi.Map{
					"interval": pulumi.String("1h"),
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
					"interval": pulumi.String("1h"),
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
				},
			},
		}

		// 4. Provision the ResourceSet CustomResource
		_, err := apiextensions.NewCustomResource(ctx, "coreaddons-resourceset",
			&apiextensions.CustomResourceArgs{
				ApiVersion: pulumi.String("fluxcd.controlplane.io/v1"),
				Kind:       pulumi.String("ResourceSet"),
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String("core-addons-generator"),
					Namespace: pulumi.String("default"),
				},
				OtherFields: map[string]any{
					"spec": pulumi.Map{
						"inputs":    inputsList,         // The structured list matrix
						"resources": resourcesTemplates, // Crucial change: 'resources' handles the template maps
					},
				},
			},
		)
		if err != nil {
			return fmt.Errorf("failed to create dynamic ResourceSet: %w", err)
		}
		return nil
	})
}
