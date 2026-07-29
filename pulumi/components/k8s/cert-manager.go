package k8s

import (
	"fmt"

	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

const (
	CERT_MANAGER_RELEASE_NAME  = "cert-manager"
	CERT_MANAGER_CHART_NAME    = "cert-manager"
	CERT_MANAGER_NAMESPACE     = "cert-manager"
	CERT_MANAGER_CHART_REPO    = "https://charts.jetstack.io"
	CERT_MANAGER_CHART_VERSION = "1.20.0"
)

type CertManagerComponent struct {
	pulumi.ResourceState
	// Endpoint pulumi.StringOutput `pulumi:"endpoint"`
}

type CertManagerArgs struct {
	InstallCrds       bool
	EnableGatewayAPI  bool
	SelfSignedCaSetup bool
}

func SetupCertManagerComponents(ctx *pulumi.Context, name string, args *CertManagerArgs, opts ...pulumi.ResourceOption) (*CertManagerComponent, error) {
	comp := &CertManagerComponent{}
	err := ctx.RegisterComponentResource("CustomComponent:k8s:CertManager", name, comp, opts...)
	if err != nil {
		return nil, err
	}

	certManagerNs, err := corev1.NewNamespace(ctx, "cert-manager-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(CERT_MANAGER_NAMESPACE),
		},
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, fmt.Errorf("Error encountered during the creation of metallb's namespace!")
	}

	cm, err := helmv3.NewRelease(
		ctx, CERT_MANAGER_RELEASE_NAME, &helmv3.ReleaseArgs{
			Chart: pulumi.String(CERT_MANAGER_CHART_NAME),
			RepositoryOpts: &helmv3.RepositoryOptsArgs{
				Repo: pulumi.String(CERT_MANAGER_CHART_REPO),
			},
			Version:   pulumi.String(CERT_MANAGER_CHART_VERSION),
			Name:      pulumi.String(CERT_MANAGER_RELEASE_NAME),
			Namespace: pulumi.String(CERT_MANAGER_NAMESPACE),
			Values: pulumi.Map{
				"crds": pulumi.Map{
					"enabled": pulumi.Bool(args.InstallCrds),
				},
				"config": pulumi.Map{
					"enableGatewayAPI": pulumi.Bool(args.EnableGatewayAPI),
				},
			},
		},
		pulumi.DependsOn([]pulumi.Resource{certManagerNs}),
		pulumi.Parent(comp),
	)
	if err != nil {
		return nil, fmt.Errorf("Error encountered during the creation of cert manager's Helm Release!")
	}

	if args.SelfSignedCaSetup {
		ssi, err := apiextensions.NewCustomResource(ctx, "selfsigned-issuer", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("cert-manager.io/v1"),
			Kind:       pulumi.String("ClusterIssuer"),
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String("selfsigned-issuer"),
			},
			OtherFields: kpulumi.UntypedArgs{
				"spec": pulumi.Map{
					"selfSigned": pulumi.Map{},
				},
			},
		},
			pulumi.DependsOn([]pulumi.Resource{cm}),
		)
		if err != nil {
			return nil, fmt.Errorf("Error encountered during the creation of self-signed issuer: %v", err)
		}

		cert, err := apiextensions.NewCustomResource(ctx, "root-ca", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("cert-manager.io/v1"),
			Kind:       pulumi.String("Certificate"),
			Metadata: &metav1.ObjectMetaArgs{
				Name:      pulumi.String("root-ca-cert"),
				Namespace: pulumi.String(CERT_MANAGER_NAMESPACE),
			},
			OtherFields: map[string]any{
				"spec": map[string]any{
					"isCA":       true,
					"commonName": "internal-root-selfsigned-ca",
					"secretName": "root-secret",
					"privateKey": map[string]any{
						"algorithm": "ECDSA",
						"size":      256,
					},
					"issuerRef": map[string]any{
						"name":  "selfsigned-issuer",
						"kind":  "ClusterIssuer",
						"group": "cert-manager.io",
					},
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{ssi}))

		if err != nil {
			return nil, fmt.Errorf("Error encountered during the creation of the self signed ca: %v", err)
		}
		_, err = apiextensions.NewCustomResource(ctx, "internal-ca-issuer", &apiextensions.CustomResourceArgs{
			ApiVersion: pulumi.String("cert-manager.io/v1"),
			Kind:       pulumi.String("ClusterIssuer"),
			Metadata: &metav1.ObjectMetaArgs{
				Name: pulumi.String("internal-ca-issuer"),
			},
			OtherFields: map[string]any{
				"spec": map[string]any{
					"ca": map[string]any{
						"secretName": "root-secret",
					},
				},
			},
		}, pulumi.DependsOn([]pulumi.Resource{cert}))
		if err != nil {
			return nil, fmt.Errorf("Error encountered during the creation of the ca issuer: %v", err)
		}
	}
	return comp, nil
}
