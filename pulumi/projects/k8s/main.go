package main

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/yaml"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type HelmConfig struct {
	ReleaseName string `json:"releaseName"`
	Chart       string `json:"chart"`
	Namespace   string `json:"namespace"`
	Repo        string `json:"repo"`
	Version     string `json:"version"`
}

func main() {
	pulumi.Run(func(ctx *pulumi.Context) error {
		cfg := config.New(ctx, "")
		kubectx := config.New(ctx, "kubernetes").Require("context")

		var nc core.NetworkComponentArgs
		cfg.RequireObject("networking", &nc)

		var g core.GatewayArgs
		cfg.RequireObject("gateway", &g)

		var h []HelmConfig
		cfg.RequireObject("helm", &h)

		// 2. Core Infrastructure Components
		networkComponent, err := core.SetupNetworkingComponents(ctx, "core-networking", &nc)
		if err != nil {
			return fmt.Errorf("networking setup failed: %w", err)
		}

		_, err = core.NewStorageClassComponent(ctx, "storageclass-setup", &core.StorageClassArgs{})
		if err != nil {
			return fmt.Errorf("storage class setup failed: %w", err)
		}

		certManager, err := core.SetupCertManagerComponents(ctx, "cert-manager", &core.CertManagerArgs{
			EnableGatewayAPI:  nc.InstallGatewayApiCrds,
			InstallCrds:       true,
			SelfSignedCaSetup: true,
		}, pulumi.DependsOn([]pulumi.Resource{networkComponent}))
		if err != nil {
			return fmt.Errorf("cert-manager setup failed: %w", err)
		}

		// Reusable core dependency handle
		infraDeps := pulumi.DependsOn([]pulumi.Resource{networkComponent, certManager})

		// 3. Cluster Gateway
		_, err = core.NewGatewayComponent(ctx, "cluster-gateway", &g, infraDeps)
		if err != nil {
			return fmt.Errorf("gateway component setup failed: %w", err)
		}

		// 4. Helm Releases (with safe override checks)
		for _, v := range h {
			overridePath := fmt.Sprintf("./clusters/%s/helm-overrides/%s/values.yaml", kubectx, v.ReleaseName)

			var valueFiles pulumi.AssetOrArchiveArray
			if _, err := os.Stat(overridePath); err == nil {
				valueFiles = pulumi.AssetOrArchiveArray{pulumi.NewFileAsset(overridePath)}
			} else if !os.IsNotExist(err) {
				return fmt.Errorf("error reading helm values override for %s: %w", v.ReleaseName, err)
			}

			_, err = helmv3.NewRelease(ctx, v.ReleaseName, &helmv3.ReleaseArgs{
				Name:            pulumi.String(v.ReleaseName),
				Chart:           pulumi.String(v.Chart),
				Namespace:       pulumi.String(v.Namespace),
				Version:         pulumi.String(v.Version),
				CreateNamespace: pulumi.Bool(true),
				RepositoryOpts: &helmv3.RepositoryOptsArgs{
					Repo: pulumi.String(v.Repo),
				},
				ValueYamlFiles: valueFiles,
			}, infraDeps)
			if err != nil {
				return fmt.Errorf("failed creating helm release '%s': %w", v.ReleaseName, err)
			}
		}

		// 5. Manifests Group (safely expands globs)
		manifestFiles, err := findManifests(fmt.Sprintf("./clusters/%s/manifests", kubectx))
		if err != nil {
			return fmt.Errorf("failed searching manifest directory: %w", err)
		}

		if len(manifestFiles) > 0 {
			_, err = yaml.NewConfigGroup(ctx, "manifests", &yaml.ConfigGroupArgs{
				Files: manifestFiles,
			}, infraDeps)
			if err != nil {
				return fmt.Errorf("failed applying manifests: %w", err)
			}
		}

		return nil
	})
}

// Helper function to resolve glob paths without throwing engine errors if empty
func findManifests(dir string) ([]string, error) {
	var files []string
	patterns := []string{"*.yaml", "*.yml"}

	for _, p := range patterns {
		matches, err := filepath.Glob(filepath.Join(dir, p))
		if err != nil {
			return nil, err
		}
		files = append(files, matches...)
	}

	return files, nil
}
