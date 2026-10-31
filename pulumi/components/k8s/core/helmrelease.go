package core

import (
	"errors"
	"fmt"

	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
)

// HelmRouteConfig optionally exposes a Service installed by the chart through
// the cluster gateway, so a release and its route live in one config entry.
type HelmRouteConfig struct {
	Hostnames []string `json:"hostnames"`
	Port      int      `json:"port"`
	// Service defaults to the release name.
	Service string `json:"service,omitempty"`
	// Namespace defaults to the release namespace.
	Namespace string `json:"namespace,omitempty"`
}

// HelmReleaseConfig is the stack-config shape for one entry of `k8s:helm`.
//
// Add a "# renovate: datasource=helm depName=<chart> registryUrl=<repo>"
// comment on the line above `version:` and Renovate will keep it current.
type HelmReleaseConfig struct {
	ReleaseName string           `json:"releaseName"`
	Chart       string           `json:"chart"`
	Repo        string           `json:"repo"`
	Version     string           `json:"version"`
	Namespace   string           `json:"namespace"`
	Route       *HelmRouteConfig `json:"route,omitempty"`
}

func (c *HelmReleaseConfig) Validate() error {
	required := []struct{ field, value string }{
		{"releaseName", c.ReleaseName},
		{"chart", c.Chart},
		{"repo", c.Repo},
		{"version", c.Version},
		{"namespace", c.Namespace},
	}
	for _, r := range required {
		if r.value == "" {
			return fmt.Errorf("%s is required", r.field)
		}
	}
	if c.Route != nil {
		if len(c.Route.Hostnames) == 0 {
			return errors.New("route.hostnames must not be empty")
		}
		if c.Route.Port < 1 || c.Route.Port > 65535 {
			return fmt.Errorf("route.port must be between 1 and 65535, got %d", c.Route.Port)
		}
	}
	return nil
}

type HelmReleaseArgs struct {
	HelmReleaseConfig

	// ValueFiles are values files, resolved by the caller. Keeping path
	// resolution out of this component means it knows nothing about how the
	// repo lays out per-cluster overrides.
	ValueFiles []string
	// Gateway is required when Route is set.
	Gateway *GatewayComponent
}

type HelmReleaseComponent struct {
	pulumi.ResourceState

	Release *helmv3.Release
}

// NewHelmReleaseComponent installs a chart (creating its namespace) and, if
// configured, routes a hostname to one of its Services.
func NewHelmReleaseComponent(
	ctx *pulumi.Context,
	name string,
	args *HelmReleaseArgs,
	opts ...pulumi.ResourceOption,
) (*HelmReleaseComponent, error) {
	if err := args.Validate(); err != nil {
		return nil, fmt.Errorf("helm release %q: %w", name, err)
	}
	if args.Route != nil && args.Gateway == nil {
		return nil, fmt.Errorf("helm release %q: a gateway is required to create its route", name)
	}

	comp := &HelmReleaseComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:HelmRelease", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register helm release component: %w", err)
	}

	var valueFiles pulumi.AssetOrArchiveArray
	for _, f := range args.ValueFiles {
		valueFiles = append(valueFiles, pulumi.NewFileAsset(f))
	}

	release, err := helmv3.NewRelease(ctx, name+"-release", &helmv3.ReleaseArgs{
		Name:            pulumi.String(args.ReleaseName),
		Chart:           pulumi.String(args.Chart),
		Version:         pulumi.String(args.Version),
		Namespace:       pulumi.String(args.Namespace),
		CreateNamespace: pulumi.Bool(true),
		Lint:            pulumi.Bool(true),
		RepositoryOpts:  &helmv3.RepositoryOptsArgs{Repo: pulumi.String(args.Repo)},
		ValueYamlFiles:  valueFiles,
	}, pulumi.Parent(comp))
	if err != nil {
		return nil, fmt.Errorf("install helm release %q: %w", args.ReleaseName, err)
	}
	comp.Release = release

	if r := args.Route; r != nil {
		routeNamespace := args.Namespace
		if r.Namespace != "" {
			routeNamespace = r.Namespace
		}
		routeService := args.ReleaseName
		if r.Service != "" {
			routeService = r.Service
		}

		if _, err := NewHTTPRouteComponent(ctx, name+"-route", &HTTPRouteArgs{
			Name:      args.ReleaseName,
			Namespace: routeNamespace,
			Hostnames: r.Hostnames,
			Gateway:   args.Gateway,
			Service:   routeService,
			Port:      r.Port,
		},
			pulumi.Parent(comp),
			// The chart creates the namespace, so the route must wait for it.
			pulumi.DependsOn([]pulumi.Resource{release, args.Gateway}),
		); err != nil {
			return nil, err
		}
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{}); err != nil {
		return nil, err
	}
	return comp, nil
}
