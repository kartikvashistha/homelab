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

	ISTIO_NAMESPACE     = "istio-system"
	ISTIO_CHART_REPO    = "https://istio-release.storage.googleapis.com/charts"
	ISTIO_CHART_VERSION = "1.29.1"

	KIALI_OPERATOR_RELEASE_NAME = "kiali-operator"
	KIALI_OPERATOR_CHART_NAME   = "kiali-operator"
	KIALI_OPERATOR_NAMESPACE    = "kiali-operator"
	KIALI_OPERATOR_REPO         = "https://kiali.org/helm-charts"
	KIALI_OPERATOR_VERSION      = "2.23.0"
)

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
	err := ctx.RegisterComponentResource("CustomComponent:k8s:Networking", name, comp, opts...)
	if err != nil {
		return nil, err
	}

	// var gatewayapicrds kustomizev2.Directory

	if args.InstallGatewayApiCrds {
		_, err := kustomizev2.NewDirectory(ctx, "gatewayapicrds", &kustomizev2.DirectoryArgs{
			// gatewayapicrds, err := kustomizev2.NewDirectory(ctx, "gatewayapicrds", &kustomizev2.DirectoryArgs{
			Directory: pulumi.String(fmt.Sprintf("github.com/kubernetes-sigs/gateway-api/config/crd?ref=%s", GATEWAYAPI_CRDS_VERSION)),
		}, pulumi.Parent(comp))
		if err != nil {
			return nil, fmt.Errorf("Error during the installation of Gateway API CRDs!")
		}
	}

	err = metallb(ctx, &args.Metallb, comp)
	if err != nil {
		return nil, err
	}

	err = setupIstio(ctx, comp)
	if err != nil {
		return nil, err
	}

	return comp, nil
}

func metallb(ctx *pulumi.Context, m *Metallb, nc *NetworkComponent) error {
	if m.Install {
		metallbNs, err := corev1.NewNamespace(ctx, "metallbns", &corev1.NamespaceArgs{
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String(METALLB_NAMESPACE),
			},
		}, pulumi.Parent(nc))
		if err != nil {
			return fmt.Errorf("Error encountered during the creation of metallb's namespace!")
		}

		metallbRelease, err := helmv3.NewRelease(
			ctx, METALLB_RELEASE_NAME, &helmv3.ReleaseArgs{
				Chart: pulumi.String(METALLB_CHART_NAME),
				RepositoryOpts: &helmv3.RepositoryOptsArgs{
					Repo: pulumi.String(METALLB_CHART_REPO),
				},
				Version:   pulumi.String(METALLB_CHART_VERSION),
				Name:      pulumi.String(METALLB_RELEASE_NAME),
				Namespace: pulumi.String(METALLB_NAMESPACE),
			},
			pulumi.DependsOn([]pulumi.Resource{metallbNs}),
			pulumi.Parent(nc),
		)
		if err != nil {
			return fmt.Errorf("Error encountered during the creation of metallb's Helm Release!")
		}

		metallbIpAddressPool, err := apiextensions.NewCustomResource(
			ctx, "metallbIpAddressPool", &apiextensions.CustomResourceArgs{
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
			},
			pulumi.DependsOn([]pulumi.Resource{metallbRelease}),
			pulumi.Parent(nc),
		)
		if err != nil {
			return err
		}

		_, err = apiextensions.NewCustomResource(
			ctx, "metallbL2Advertisement", &apiextensions.CustomResourceArgs{
				ApiVersion: pulumi.String("metallb.io/v1beta1"),
				Kind:       pulumi.String("L2Advertisement"),
				Metadata: &metav1.ObjectMetaArgs{
					Name:      pulumi.String("advertisement"),
					Namespace: pulumi.String("metallb-system"),
				},
			},
			pulumi.DependsOn([]pulumi.Resource{metallbIpAddressPool}),
			pulumi.Parent(nc),
		)
		if err != nil {
			return err
		}
		return nil
	}

	return nil
}

// This function sets up the various istio components for:
// (1) Service mesh
// (2) GatewayClass &
// (3) More istio stuff that I dont fully understand yet
// (4) Kiali Operator
func setupIstio(ctx *pulumi.Context, nc *NetworkComponent) error {
	istioNs, err := corev1.NewNamespace(ctx, "istio-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(ISTIO_NAMESPACE),
		},
	}, pulumi.Parent(nc))
	if err != nil {
		return err
	}

	_, err = helmv3.NewRelease(ctx, "istio-cni", &helmv3.ReleaseArgs{
		Chart: pulumi.String("cni"),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String(ISTIO_CHART_REPO),
		},
		Name:      pulumi.String("istio-cni"),
		Namespace: pulumi.String(ISTIO_NAMESPACE),
		Version:   pulumi.String(ISTIO_CHART_VERSION),
		Values: pulumi.Map{
			"ambient": pulumi.Map{
				"enabled": pulumi.Bool(true),
			},
			"seLinuxOptions": pulumi.Map{
				"type": pulumi.String("spc_t"),
			},
		},
	},
		pulumi.Parent(nc),
		pulumi.DependsOn([]pulumi.Resource{istioNs}))
	if err != nil {
		return err
	}

	_, err = helmv3.NewRelease(ctx, "istiod", &helmv3.ReleaseArgs{
		Chart: pulumi.String("istiod"),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String(ISTIO_CHART_REPO),
		},
		Name:      pulumi.String("istiod"),
		Namespace: pulumi.String(ISTIO_NAMESPACE),
		Version:   pulumi.String(ISTIO_CHART_VERSION),
		Values: pulumi.Map{
			"env": pulumi.Map{
				"PILOT_ENABLE_AMBIENT": pulumi.Bool(true),
			},
		},
	},
		pulumi.Parent(nc),
		pulumi.DependsOn([]pulumi.Resource{istioNs}))
	if err != nil {
		return err
	}

	_, err = helmv3.NewRelease(ctx, "istio-base", &helmv3.ReleaseArgs{
		Chart: pulumi.String("base"),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String(ISTIO_CHART_REPO),
		},
		Name:      pulumi.String("istio-base"),
		Namespace: pulumi.String(ISTIO_NAMESPACE),
		Version:   pulumi.String(ISTIO_CHART_VERSION),
	},
		pulumi.Parent(nc),
		pulumi.DependsOn([]pulumi.Resource{istioNs}))
	if err != nil {
		return err
	}

	_, err = helmv3.NewRelease(ctx, "ztunnel", &helmv3.ReleaseArgs{
		Chart: pulumi.String("ztunnel"),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String(ISTIO_CHART_REPO),
		},
		Name:      pulumi.String("ztunnel"),
		Namespace: pulumi.String(ISTIO_NAMESPACE),
		Version:   pulumi.String(ISTIO_CHART_VERSION),
		Values: pulumi.Map{
			"seLinuxOptions": pulumi.Map{
				"type": pulumi.String("spc_t"),
			},
		},
	}, pulumi.Parent(nc), pulumi.DependsOn([]pulumi.Resource{istioNs}))

	kialiOperatorNs, err := corev1.NewNamespace(ctx, KIALI_OPERATOR_NAMESPACE+"-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(KIALI_OPERATOR_RELEASE_NAME),
		},
	}, pulumi.Parent(nc))
	if err != nil {
		return err
	}
	_, err = helmv3.NewRelease(ctx, "kiali-operator", &helmv3.ReleaseArgs{
		Chart: pulumi.String(KIALI_OPERATOR_CHART_NAME),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{
			Repo: pulumi.String(KIALI_OPERATOR_REPO),
		},
		Name:      pulumi.String(KIALI_OPERATOR_RELEASE_NAME),
		Namespace: pulumi.String(KIALI_OPERATOR_RELEASE_NAME),
		Version:   pulumi.String(KIALI_OPERATOR_VERSION),
		Values: pulumi.Map{
			"cr": pulumi.Map{
				"create":    pulumi.Bool(true),
				"namespace": pulumi.String(ISTIO_NAMESPACE),
				"spec": pulumi.Map{
					"auth": pulumi.Map{
						"strategy": pulumi.String("anonymous"),
					},
				},
			},
		},
	},
		pulumi.Parent(nc),
		pulumi.DependsOn([]pulumi.Resource{istioNs, kialiOperatorNs}))

	return nil
}
