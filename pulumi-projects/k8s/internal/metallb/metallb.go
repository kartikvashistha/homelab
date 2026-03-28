package metallb

import (
	"fmt"
	"k8s/internal/helm"

	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	METALLB_RELEASE_NAME  = "metallb"
	METALLB_CHART_NAME    = "metallb"
	METALLB_NAMESPACE     = "metallb-system"
	METALLB_CHART_REPO    = "https://metallb.github.io/metallb"
	METALLB_CHART_VERSION = "0.15.3"
)

type Metallb struct {
	Install     bool
	AddressPool []string
}

func BootstrapMetallb(ctx *pulumi.Context, m Metallb) error {
	metallbRelease, err := helm.CreateHelmRelease(ctx, helm.HelmChart{
		Chart:       METALLB_CHART_NAME,
		Repo:        METALLB_CHART_REPO,
		Version:     METALLB_CHART_VERSION,
		ReleaseName: METALLB_RELEASE_NAME,
		Namespace:   METALLB_NAMESPACE,
	})
	if err != nil {
		fmt.Println("Error encountered during metallb installalation!")
		return err
	}

	_, err = apiextensions.NewCustomResource(ctx, "metallbIpAddressPool", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("metallb.io/v1beta1"),
		Kind:       pulumi.String("IPAddressPool"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("first-pool"),
			Namespace: pulumi.String("metallb-system"),
		},
		OtherFields: kpulumi.UntypedArgs{
			"spec": pulumi.Map{
				"addresses": pulumi.ToStringArray(m.AddressPool),
			},
		},
	}, pulumi.DependsOn([]pulumi.Resource{metallbRelease}))

	if err != nil {
		return err
	}

	_, err = apiextensions.NewCustomResource(ctx, "metallbIpAddressPool", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("metallb.io/v1beta1"),
		Kind:       pulumi.String("L2Advertisement"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("advertisement"),
			Namespace: pulumi.String("metallb-system"),
		},
	}, pulumi.DependsOn([]pulumi.Resource{metallbRelease}))

	if err != nil {
		return err
	}

	return nil
}
