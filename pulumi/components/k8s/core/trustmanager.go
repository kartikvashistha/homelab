package core

import (
	"fmt"

	kpulumi "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/apiextensions"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

const (
	// InternalCAConfigMap is the name of the ConfigMap trust-manager creates
	// in every namespace; InternalCAKey is the key holding the PEM. Mount it
	// into workloads that must trust services signed by the internal CA:
	//
	//	volumes:      [{name: internal-ca, configMap: {name: internal-ca}}]
	//	volumeMounts: [{name: internal-ca, mountPath: /etc/ssl/certs/internal-ca.crt, subPath: ca.crt, readOnly: true}]
	InternalCAConfigMap = "internal-ca"
	InternalCAKey       = "ca.crt"
)

// TrustManagerComponent installs trust-manager and publishes the internal root
// CA to every namespace as the InternalCAConfigMap ConfigMap. It tracks the
// CA's rotation and covers namespaces created later, so apps (Headlamp, Kiali,
// ...) can trust the CA without per-namespace plumbing.
type TrustManagerComponent struct {
	pulumi.ResourceState
}

func NewTrustManagerComponent(
	ctx *pulumi.Context,
	name string,
	certManager *CertManagerComponent,
	opts ...pulumi.ResourceOption,
) (*TrustManagerComponent, error) {
	comp := &TrustManagerComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:TrustManager", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register trust-manager component: %w", err)
	}
	parentOpt := pulumi.Parent(comp)

	// Depending on the cert-manager component waits for all its children,
	// including the root CA Certificate and therefore its Secret.
	certManagerDep := pulumi.DependsOn([]pulumi.Resource{certManager})

	release, err := helmv3.NewRelease(ctx, name, &helmv3.ReleaseArgs{
		Name:      pulumi.String("trust-manager"),
		Chart:     pulumi.String(versions.TrustManagerChart),
		Version:   pulumi.String(versions.TrustManager),
		Namespace: pulumi.String(certManagerNamespace),
		Values: pulumi.Map{
			// Source Secrets are only read from the trust namespace, which
			// is where cert-manager keeps the root CA's Secret.
			"app": pulumi.Map{
				"trust": pulumi.Map{"namespace": pulumi.String(certManagerNamespace)},
			},
		},
	}, parentOpt, certManagerDep)
	if err != nil {
		return nil, fmt.Errorf("install trust-manager: %w", err)
	}

	// A Bundle is cluster-scoped. With no namespaceSelector its target
	// ConfigMap (named after the Bundle) is synced into every namespace.
	// The CA certificate lives in tls.crt: it is the CA's own certificate.
	_, err = apiextensions.NewCustomResource(ctx, "internal-ca-bundle", &apiextensions.CustomResourceArgs{
		ApiVersion: pulumi.String("trust.cert-manager.io/v1alpha1"),
		Kind:       pulumi.String("Bundle"),
		Metadata:   &metav1.ObjectMetaArgs{Name: pulumi.String(InternalCAConfigMap)},
		OtherFields: kpulumi.UntypedArgs{
			"spec": pulumi.Map{
				"sources": pulumi.MapArray{
					pulumi.Map{
						"secret": pulumi.Map{
							"name": pulumi.String(rootCASecret),
							"key":  pulumi.String("tls.crt"),
						},
					},
				},
				"target": pulumi.Map{
					"configMap": pulumi.Map{"key": pulumi.String(InternalCAKey)},
				},
			},
		},
	}, parentOpt, pulumi.DependsOn([]pulumi.Resource{release}))
	if err != nil {
		return nil, fmt.Errorf("create internal CA bundle: %w", err)
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{}); err != nil {
		return nil, err
	}
	return comp, nil
}
