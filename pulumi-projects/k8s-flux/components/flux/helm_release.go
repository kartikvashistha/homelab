package flux

import (
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// ResourceSetArgs groups HelmChart & HelmRelease configurations
type ResourceSetArgs struct {
	// Release (Helm) Configurations
	ChartName        pulumi.StringInput // Name of the chart inside the OCI artifact
	ChartVersion     pulumi.StringInput
	ReleaseName      pulumi.StringInput // Name of the final HelmRelease
	ReleaseNamespace pulumi.StringInput
	RepoName         pulumi.StringInput
	RepoUrl          pulumi.StringInput
	Values           pulumi.MapInput // Deployment overrides
}

type ResourceSet struct {
	pulumi.ResourceState
}

func HelmResourceSet(ctx *pulumi.Context, name string, args *ResourceSetArgs, opts ...pulumi.ResourceOption) (*ResourceSet, error) {
	component := &ResourceSet{}
	err := ctx.RegisterComponentResource("custom:flux:ResourceSet", name, component, opts...)
	if err != nil {
		return nil, err
	}

	childOpts := append(opts, pulumi.Parent(component))

	releaseNs, err := corev1.NewNamespace(ctx, name+"-release-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: args.ReleaseNamespace,
		},
	}, childOpts...)
	if err != nil {
		return nil, err
	}

	// Fix 1: Map your input struct explicitly to match the exact keys expected by your template
	inputsMap := pulumi.Array{pulumi.Map{
		"chartName":    args.ChartName,
		"chartVersion": args.ChartVersion,
		"releaseName":  args.ReleaseName,
		"namespace":    args.ReleaseNamespace,
		"repoName":     args.RepoName,
		"repoUrl":      args.RepoUrl,
		"values":       args.Values,
	}}

	// Fix 2: Changed from non-existent `pulumi.MapArray` to `pulumi.Array`
	resourcesTemplates := pulumi.Array{
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

	// Fix 3: Map custom spec to 'OtherFields' matching map[string]pulumi.Input type signatures correctly
	_, err = apiextensions.NewCustomResource(ctx, name+"_helmReleaseResourceSet",
		&apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("fluxcd.controlplane.io/v1"),
			Kind:       pulumi.String("ResourceSet"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String(name + "-helm-release-generator"),
				Namespace: pulumi.String("default"),
				Annotations: pulumi.StringMap{
					"fluxcd.controlplane.io/reconcile":      pulumi.String("enabled"),
					"fluxcd.controlplane.io/reconcileEvery": pulumi.String("10m"),
				},
			},
			OtherFields: map[string]any{
				"spec": pulumi.Map{
					"inputs":    inputsMap,
					"resources": resourcesTemplates,
				},
			},
		}, append(childOpts, pulumi.DependsOn([]pulumi.Resource{releaseNs}))...)

	if err != nil {
		return nil, err
	}

	if err := ctx.RegisterResourceOutputs(component, pulumi.Map{}); err != nil {
		return nil, err
	}

	return component, nil
}
