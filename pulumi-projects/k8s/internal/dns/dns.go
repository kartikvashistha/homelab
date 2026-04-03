package dns

import (
	"fmt"
	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"k8s/internal/helm"
)

const (
	COREDNS_RELEASE_NAME  = "coredns-external"
	COREDNS_CHART_NAME    = "coredns"
	COREDNS_NAMESPACE     = "coredns"
	COREDNS_CHART_REPO    = "https://coredns.github.io/helm"
	COREDNS_CHART_VERSION = "1.45.2"
)

func BootstrapDnsResolver(ctx *pulumi.Context) error {

	corednsNS, err := corev1.NewNamespace(ctx, "coredns-external-namespace", &corev1.NamespaceArgs{
		ApiVersion: pulumi.String("string"),
		Kind:       pulumi.String("string"),
		Metadata: &metav1.ObjectMetaArgs{
			Name: pulumi.String(COREDNS_NAMESPACE)},
	})
	if err != nil {
		fmt.Printf("Error during the creation of namespace:%v", COREDNS_NAMESPACE)
		return err
	}

	_, err = yaml.NewConfigGroup(ctx, "cert-manager-self-signed-issuer-setup",
		&yaml.ConfigGroupArgs{
			Files: []string{"./cfg-private-dns-hosts.yaml"},
		}, pulumi.DependsOn([]pulumi.Resource{corednsNS}))
	if err != nil {
		fmt.Println("Error occured during creation of configmap: private-dns")
		return err
	}

	_, err = helm.CreateHelmRelease(ctx, helm.HelmChart{
		Chart:       COREDNS_CHART_NAME,
		Repo:        COREDNS_CHART_REPO,
		Version:     COREDNS_CHART_VERSION,
		ReleaseName: COREDNS_RELEASE_NAME,
		Namespace:   COREDNS_NAMESPACE,
		ValuesFile:  "./coredns-value-overrides.yaml",
	})

	if err != nil {
		fmt.Println("Error encountered during coredns installalation!")
		return err
	}
	return nil
}
