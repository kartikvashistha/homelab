package main

import (
	"fmt"
	"maps"
	"slices"

	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/apps"
	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
)

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		k8s := config.New(ctx, "k8s")

		sc, err := loadConfig(ctx, k8s)
		if err != nil {
			return err
		}

		return deploy(ctx, k8s, sc)
	})
}

// deploy wires config to components, in dependency order:
// platform -> DNS -> helm releases -> raw manifests -> external services -> apps.
func deploy(ctx *pulumi.Context, k8s *config.Config, sc *stackConfig) error {
	cluster := newClusterDir(sc.Cluster)

	// 1. Platform: networking, certificates, storage, and the gateway.
	platform, err := core.NewPlatform(ctx, "platform", &sc.Platform)
	if err != nil {
		return fmt.Errorf("platform: %w", err)
	}
	infra := platform.Infra()

	// 2. DNS. Every name under the gateway domain resolves to the gateway.
	if sc.CoreDNS != nil {
		if _, err := core.NewCoreDNSComponent(ctx, "homedns", &core.CoreDNSArgs{
			CoreDNSConfig: *sc.CoreDNS,
			Domain:        sc.Platform.Gateway.ZoneDomain(),
			GatewayIP:     sc.Platform.Gateway.GatewayIp,
		}, pulumi.DependsOn([]pulumi.Resource{platform.Networking})); err != nil {
			return fmt.Errorf("coredns: %w", err)
		}
	}

	// 3. Helm releases, with optional per-cluster values overrides.
	for _, h := range sc.Helm {
		valueFiles, err := cluster.helmValueFiles(h.ReleaseName)
		if err != nil {
			return err
		}

		if _, err := core.NewHelmReleaseComponent(ctx, h.ReleaseName, &core.HelmReleaseArgs{
			HelmReleaseConfig: h,
			ValueFiles:        valueFiles,
			Gateway:           platform.Gateway,
		}, infra); err != nil {
			return err
		}
	}

	// 4. Raw manifests from clusters/<cluster>/manifests.
	manifests, err := cluster.manifests()
	if err != nil {
		return fmt.Errorf("find manifests: %w", err)
	}
	if len(manifests) > 0 {
		if _, err := yaml.NewConfigGroup(ctx, "manifests", &yaml.ConfigGroupArgs{
			Files: manifests,
		}, infra); err != nil {
			return fmt.Errorf("apply manifests: %w", err)
		}
	}

	// 5. LAN services fronted by the gateway. Sorted for a stable
	// registration order.
	for _, name := range slices.Sorted(maps.Keys(sc.ExternalServices)) {
		if _, err := core.NewExternalServiceComponent(ctx, name, &core.ExternalServiceArgs{
			ExternalServiceConfig: sc.ExternalServices[name],
			Gateway:               platform.Gateway,
		}); err != nil {
			return err
		}
	}

	// 6. Opt-in in-cluster apps.
	if err := apps.Deploy(ctx, k8s, platform.Gateway); err != nil {
		return fmt.Errorf("apps: %w", err)
	}

	return nil
}
