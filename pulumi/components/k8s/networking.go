package k8s

import (
	"fmt"
	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	kustomizev2 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/kustomize/v2"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	GATEWAYAPI_CRDS_VERSION = "v1.4.0"

	METALLB_RELEASE_NAME  = "metallb"
	METALLB_CHART_NAME    = "metallb"
	METALLB_NAMESPACE     = "metallb-system"
	METALLB_CHART_REPO    = "https://metallb.github.io/metallb"
	METALLB_CHART_VERSION = "0.15.3"
)

// This component should:
// 1. Install Gateway API CRD's
// 2. Install & configure metallb
// 3. Install & configure Istio Components
type NetworkComponent struct {
	pulumi.ResourceState
	// Endpoint pulumi.StringOutput `pulumi:"endpoint"`
}

type Metallb struct {
	Install     bool
	AddressPool []string
}

type NetworkComponentArgs struct {
	InstallGatewayApiCrds bool
	Metallb               Metallb
}

func SetupNetworkingComponents(ctx *pulumi.Context, name string, args *NetworkComponentArgs, opts ...pulumi.ResourceOption) (*NetworkComponent, error) {
	comp := &NetworkComponent{}

	// var gatewayapicrds kustomizev2.Directory

	if args.InstallGatewayApiCrds {
		_, err := kustomizev2.NewDirectory(ctx, "gatewayapicrds", &kustomizev2.DirectoryArgs{
			// gatewayapicrds, err := kustomizev2.NewDirectory(ctx, "gatewayapicrds", &kustomizev2.DirectoryArgs{
			Directory: pulumi.String(fmt.Sprintf("github.com/kubernetes-sigs/gateway-api/config/crd?ref=%s", GATEWAYAPI_CRDS_VERSION))})

		if err != nil {
			return nil, fmt.Errorf("Error during the installation of Gateway API CRDs!")
		}
	}

	if args.Metallb.Install {
		metallbNs, err := corev1.NewNamespace(ctx, "metallbns", &corev1.NamespaceArgs{
			// ApiVersion: pulumi.String("v1"),
			// Kind:       pulumi.String("string"),
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(METALLB_NAMESPACE),
			},
		})

		if err != nil {
			return nil, fmt.Errorf("Error encountered during the creation of metallb's namespace!")
		}

		metallbRelease, err := helmv3.NewRelease(ctx, METALLB_RELEASE_NAME, &helmv3.ReleaseArgs{
			Chart: pulumi.String(METALLB_CHART_NAME),
			RepositoryOpts: &helmv3.RepositoryOptsArgs{
				Repo: pulumi.String(METALLB_CHART_REPO),
			},
			Version:   pulumi.String(METALLB_CHART_VERSION),
			Name:      pulumi.String(METALLB_RELEASE_NAME),
			Namespace: pulumi.String(METALLB_NAMESPACE),
		}, pulumi.DependsOn([]pulumi.Resource{metallbNs}))

		if err != nil {
			return nil, fmt.Errorf("Error encountered during the creation of metallb's Helm Release!")
		}

		metallbIpAddressPool, err := apiextensions.NewCustomResource(ctx, "metallbIpAddressPool", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("metallb.io/v1beta1"),
			Kind:       pulumi.String("IPAddressPool"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("first-pool"),
				Namespace: pulumi.String("metallb-system"),
			},
			OtherFields: kpulumi.UntypedArgs{
				"spec": pulumi.Map{
					"addresses": pulumi.ToStringArray(args.Metallb.AddressPool),
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{metallbRelease}))

		if err != nil {
			return nil, err
		}

		_, err = apiextensions.NewCustomResource(ctx, "metallbL2Advertisement", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("metallb.io/v1beta1"),
			Kind:       pulumi.String("L2Advertisement"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("advertisement"),
				Namespace: pulumi.String("metallb-system"),
			},
		}, pulumi.DependsOn([]pulumi.Resource{metallbIpAddressPool}))

		if err != nil {
			return nil, err
		}
	}

	return comp, nil
}
