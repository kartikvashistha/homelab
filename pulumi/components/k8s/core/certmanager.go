package core

import (
	"fmt"

	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

const (
	certManagerNamespace = "cert-manager"
	internalCAIssuer     = "internal-ca-issuer"
	rootCASecret         = "root-secret"
)

type CertManagerArgs struct {
	InstallCrds      bool
	EnableGatewayAPI bool
	// SelfSignedCA bootstraps a private root CA and a ClusterIssuer backed by it.
	SelfSignedCA bool
}

type CertManagerComponent struct {
	pulumi.ResourceState

	// ClusterIssuer is the name of the ClusterIssuer other components should
	// reference for certificates. Empty when SelfSignedCA is disabled.
	ClusterIssuer string
}

func NewCertManagerComponent(
	ctx *pulumi.Context,
	name string,
	args *CertManagerArgs,
	opts ...pulumi.ResourceOption,
) (*CertManagerComponent, error) {
	comp := &CertManagerComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:CertManager", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register cert-manager component: %w", err)
	}
	parentOpt := pulumi.Parent(comp)

	ns, err := corev1.NewNamespace(ctx, "cert-manager-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(certManagerNamespace)},
	}, parentOpt)
	if err != nil {
		return nil, fmt.Errorf("create cert-manager namespace: %w", err)
	}

	release, err := helmv3.NewRelease(ctx, "cert-manager", &helmv3.ReleaseArgs{
		Name:           pulumi.String("cert-manager"),
		Chart:          pulumi.String("cert-manager"),
		Namespace:      ns.Metadata.Name(),
		Version:        pulumi.String(versions.CertManager),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(versions.CertManagerRepo)},
		Values: pulumi.Map{
			"crds":   pulumi.Map{"enabled": pulumi.Bool(args.InstallCrds)},
			"config": pulumi.Map{"enableGatewayAPI": pulumi.Bool(args.EnableGatewayAPI)},
		},
	}, parentOpt)
	if err != nil {
		return nil, fmt.Errorf("install cert-manager: %w", err)
	}

	if args.SelfSignedCA {
		if err := newSelfSignedCA(ctx, ns, release, parentOpt); err != nil {
			return nil, err
		}
		comp.ClusterIssuer = internalCAIssuer
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{
		"clusterIssuer": pulumi.String(comp.ClusterIssuer),
	}); err != nil {
		return nil, err
	}
	return comp, nil
}

// newSelfSignedCA creates the usual bootstrap chain:
//
//	self-signed ClusterIssuer -> root CA Certificate -> CA ClusterIssuer
func newSelfSignedCA(
	ctx *pulumi.Context,
	ns *corev1.Namespace,
	release *helmv3.Release,
	parentOpt pulumi.ResourceOption,
) error {
	selfSigned, err := apiextensions.NewCustomResource(ctx, "selfsigned-issuer", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("cert-manager.io/v1"),
		Kind:       pulumi.String("ClusterIssuer"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String("selfsigned-issuer")},
		OtherFields: kpulumi.UntypedArgs{
			"spec": pulumi.Map{"selfSigned": pulumi.Map{}},
		},
	}, parentOpt, pulumi.DependsOn([]pulumi.Resource{release}))
	if err != nil {
		return fmt.Errorf("create self-signed issuer: %w", err)
	}

	rootCA, err := apiextensions.NewCustomResource(ctx, "root-ca", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("cert-manager.io/v1"),
		Kind:       pulumi.String("Certificate"),
		Metadata: &metav1.ObjectMetaArgs{
			Name:      pulumi.String("root-ca-cert"),
			Namespace: ns.Metadata.Name(),
		},
		OtherFields: kpulumi.UntypedArgs{
			"spec": pulumi.Map{
				"isCA":       pulumi.Bool(true),
				"commonName": pulumi.String("internal-root-selfsigned-ca"),
				"secretName": pulumi.String(rootCASecret),
				// 10 years. The default is 90 days, which for a root CA you
				// import into devices means re-trusting it every quarter.
				"duration": pulumi.String("87600h"),
				"privateKey": pulumi.Map{
					"algorithm": pulumi.String("ECDSA"),
					"size":      pulumi.Int(256),
				},
				"issuerRef": pulumi.Map{
					"name":  pulumi.String("selfsigned-issuer"),
					"kind":  pulumi.String("ClusterIssuer"),
					"group": pulumi.String("cert-manager.io"),
				},
			},
		},
	}, parentOpt, pulumi.DependsOn([]pulumi.Resource{selfSigned}))
	if err != nil {
		return fmt.Errorf("create root CA certificate: %w", err)
	}

	if _, err := apiextensions.NewCustomResource(ctx, "internal-ca-issuer", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("cert-manager.io/v1"),
		Kind:       pulumi.String("ClusterIssuer"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String(internalCAIssuer)},
		OtherFields: kpulumi.UntypedArgs{
			"spec": pulumi.Map{
				"ca": pulumi.Map{"secretName": pulumi.String(rootCASecret)},
			},
		},
	}, parentOpt, pulumi.DependsOn([]pulumi.Resource{rootCA})); err != nil {
		return fmt.Errorf("create internal CA issuer: %w", err)
	}

	return nil
}
