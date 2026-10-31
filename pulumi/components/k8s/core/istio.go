package core

import (
	"fmt"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

// IstioNamespace is where the Istio control plane (and the cluster Gateway)
// live.
const IstioNamespace = "istio-system"

// newIstio installs Istio in ambient mode, in the order Istio documents:
//
//	base -> istiod -> cni -> ztunnel
//
// Each release depends on the previous one so a slow or failing step blocks
// the steps that need it, instead of racing.
func newIstio(ctx *pulumi.Context, parent pulumi.Resource) error {
	ns, err := corev1.NewNamespace(ctx, "istio-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(IstioNamespace)},
	}, pulumi.Parent(parent))
	if err != nil {
		return fmt.Errorf("create istio namespace: %w", err)
	}

	install := func(name, chart string, values pulumi.Map, deps ...pulumi.Resource) (*helmv3.Release, error) {
		releaseArgs := &helmv3.ReleaseArgs{
			Name:           pulumi.String(name),
			Chart:          pulumi.String(chart),
			Namespace:      ns.Metadata.Name(),
			Lint:           pulumi.Bool(true),
			Version:        pulumi.String(versions.Istio),
			RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(versions.IstioRepo)},
		}
		if len(values) > 0 {
			releaseArgs.Values = values
		}

		release, err := helmv3.NewRelease(ctx, name, releaseArgs,
			pulumi.Parent(parent),
			pulumi.DependsOn(append([]pulumi.Resource{ns}, deps...)),
		)
		if err != nil {
			return nil, fmt.Errorf("install %s: %w", name, err)
		}
		return release, nil
	}

	seLinux := pulumi.Map{"type": pulumi.String("spc_t")}

	base, err := install("istio-base", "base", nil)
	if err != nil {
		return err
	}

	istiod, err := install("istiod", "istiod", pulumi.Map{
		"env": pulumi.Map{"PILOT_ENABLE_AMBIENT": pulumi.Bool(true)},
	}, base)
	if err != nil {
		return err
	}

	cni, err := install("istio-cni", "cni", pulumi.Map{
		"ambient":        pulumi.Map{"enabled": pulumi.Bool(true)},
		"seLinuxOptions": seLinux,
	}, istiod)
	if err != nil {
		return err
	}

	if _, err := install("ztunnel", "ztunnel", pulumi.Map{
		"seLinuxOptions": seLinux,
	}, istiod, cni); err != nil {
		return err
	}

	return nil
}
