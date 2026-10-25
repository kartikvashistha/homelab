package main

import (
	"cmp"
	"fmt"

	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
)

// stackConfig is everything the stack file configures, loaded and typed in one
// place. Validation of each section lives with the component that consumes it,
// so errors surface during preview before any resource is registered.
//
// (Pulumi.yaml's config schema can't describe structured values, so this file
// and the component config structs are the schema.)
type stackConfig struct {
	// Cluster selects clusters/<cluster>/. Defaults to the kube context name.
	Cluster string

	Platform         core.PlatformArgs
	CoreDNS          *core.CoreDNSConfig // nil: don't install CoreDNS
	Helm             []core.HelmReleaseConfig
	ExternalServices map[string]core.ExternalServiceConfig
}

func loadConfig(ctx *pulumi.Context, k8s *config.Config) (*stackConfig, error) {
	kubeContext := config.New(ctx, "kubernetes").Require("context")

	sc := &stackConfig{Cluster: cmp.Or(k8s.Get("cluster"), kubeContext)}

	for key, out := range map[string]any{
		"sso":              &sc.Platform.Sso,
		"networking":       &sc.Platform.Networking,
		"storage":          &sc.Platform.Storage,
		"gateway":          &sc.Platform.Gateway,
		"coredns":          &sc.CoreDNS,
		"helm":             &sc.Helm,
		"externalServices": &sc.ExternalServices,
	} {
		if err := k8s.GetObject(key, out); err != nil {
			return nil, fmt.Errorf("read k8s:%s config: %w", key, err)
		}
	}

	if err := loadSSOSecrets(k8s, &sc.Platform.Sso); err != nil {
		return nil, err
	}

	for i := range sc.Helm {
		if err := sc.Helm[i].Validate(); err != nil {
			return nil, fmt.Errorf("k8s:helm[%d]: %w", i, err)
		}
	}

	return sc, nil
}

// loadSSOSecrets reads Authentik's secrets from stack config. They're meant to
// come from a Pulumi ESC environment (see esc/dreadnought.example.yaml), which
// exposes them as secret config:
//
//	k8s:authentik-secret-key
//	k8s:authentik-pg-password
//	k8s:oidc-secret-<client slug>      one per entry of k8s:sso clients
//
// They stay secret outputs end to end, so they're encrypted in state and
// masked in previews.
func loadSSOSecrets(k8s *config.Config, sso *core.AuthentikArgs) error {
	var err error
	if sso.SecretKey, err = k8s.TrySecret("authentik-secret-key"); err != nil {
		return fmt.Errorf("k8s:authentik-secret-key: %w", err)
	}
	if sso.PostgresPassword, err = k8s.TrySecret("authentik-pg-password"); err != nil {
		return fmt.Errorf("k8s:authentik-pg-password: %w", err)
	}

	sso.ClientSecrets = make(map[string]pulumi.StringInput, len(sso.Clients))
	for _, c := range sso.Clients {
		key := "oidc-secret-" + c.Slug
		secret, err := k8s.TrySecret(key)
		if err != nil {
			return fmt.Errorf("k8s:%s: %w", key, err)
		}
		sso.ClientSecrets[c.Slug] = secret
	}
	return nil
}
