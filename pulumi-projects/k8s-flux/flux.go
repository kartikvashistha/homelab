package main

import (
	"fmt"
	"k8s-flux/internal/helm"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	FLUX_OPERATOR_RELEASE_NAME  = "flux-operator"
	FLUX_OPERATOR_NAMESPACE     = "flux-system"
	FLUX_OPERATOR_CHART         = "oci://ghcr.io/controlplaneio-fluxcd/charts/flux-operator"
	FLUX_OPERATOR_CHART_VERSION = "0.53.0"
)

func setupFluxOperator(ctx *pulumi.Context) error {
	fluxOperatorHelmRelease, err := helm.CreateHelmReleaseFromOCI(ctx, helm.HelmChart{
		Chart:       FLUX_OPERATOR_CHART,
		Namespace:   FLUX_OPERATOR_NAMESPACE,
		ReleaseName: FLUX_OPERATOR_RELEASE_NAME,
		Version:     FLUX_OPERATOR_CHART_VERSION,
	})
	if err != nil {
		fmt.Println("Error encountered during Flux Operator Helm release creation!")
		return err
	}

	_, err = apiextensions.NewCustomResource(ctx, "flux-instance",
		&apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("fluxcd.controlplane.io/v1"),
			Kind:       pulumi.String("FluxInstance"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("flux"),
				Namespace: pulumi.String(FLUX_OPERATOR_NAMESPACE),
			},
			OtherFields: map[string]any{
				"spec": pulumi.Map{
					"distribution": pulumi.Map{
						"version":  pulumi.String("2.8.x"),
						"registry": pulumi.String("ghcr.io/fluxcd"),
					},
					"cluster": pulumi.Map{
						"type": pulumi.String("kubernetes"),
						"size": pulumi.String("medium"),
					},
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{fluxOperatorHelmRelease}),
	)
	return nil
}
