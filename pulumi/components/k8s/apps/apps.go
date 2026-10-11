package apps

import (
	"bytes"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

// registry lists every app this project knows how to deploy. Adding an app is
// one new file implementing App plus one line here; nothing else changes.
var registry = []App{
	KialiApp{},
}

// App is a deployable unit selected by a key under the `k8s:apps` stack config.
// Each app decodes and validates its own config, so the registry stays generic
// and apps stay type-safe.
type App interface {
	Name() string
	Deploy(ctx *pulumi.Context, gateway *core.GatewayComponent, raw json.RawMessage) error
}

// Deploy deploys every registered app that has an entry under `k8s:apps`.
// Entries that match no registered app are an error rather than silently
// ignored, so a typo fails in preview instead of deploying nothing.
func Deploy(ctx *pulumi.Context, cfg *config.Config, gateway *core.GatewayComponent) error {
	var enabled map[string]json.RawMessage
	if err := cfg.GetObject("apps", &enabled); err != nil {
		return fmt.Errorf("read apps config: %w", err)
	}

	known := make([]string, 0, len(registry))
	for _, app := range registry {
		known = append(known, app.Name())
	}

	for name := range enabled {
		if !slices.Contains(known, name) {
			return fmt.Errorf(
				"unknown app %q in k8s:apps (known apps: %s)",
				name, strings.Join(known, ", "),
			)
		}
	}

	for _, app := range registry {
		raw, ok := enabled[app.Name()]
		if !ok {
			continue
		}
		if err := app.Deploy(ctx, gateway, raw); err != nil {
			return fmt.Errorf("deploy %s: %w", app.Name(), err)
		}
	}

	return nil
}

// decode unmarshals an app's raw config into T, rejecting unknown fields so
// that misspelled keys are reported instead of dropped.
func decode[T any](raw json.RawMessage) (T, error) {
	var out T
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&out); err != nil {
		return out, err
	}
	return out, nil
}
