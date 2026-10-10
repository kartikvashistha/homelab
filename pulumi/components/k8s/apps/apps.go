package apps

import (
	"encoding/json"
	"fmt"
	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

var registry = []App{
	KialiApp{},
	JellyfinApp{},
}

type AppsConfig struct {
	Kiali    KialiArgs    `json:"kiali"`
	Jellyfin JellyfinArgs `json:"jellyfin"`
}

type App interface {
	Name() string
	Deploy(
		ctx *pulumi.Context,
		gateway *core.GatewayComponent,
		args any,
	) error
}

func Deploy(ctx *pulumi.Context, cfg *config.Config, gateway *core.GatewayComponent) error {
	var raw map[string]json.RawMessage
	if err := cfg.GetObject("apps", &raw); err != nil {
		return fmt.Errorf("read apps config: %w", err)
	}

	var appsCfg AppsConfig
	if err := cfg.GetObject("apps", &appsCfg); err != nil {
		return fmt.Errorf("decode apps config: %w", err)
	}

	for _, app := range registry {
		switch app.Name() {
		case "kiali":
			if _, ok := raw["kiali"]; ok {
				if err := app.Deploy(ctx, gateway, appsCfg.Kiali); err != nil {
					return fmt.Errorf("deploy kiali: %w", err)
				}
			}
		case "jellyfin":
			if _, ok := raw["jellyfin"]; ok {
				if err := app.Deploy(ctx, gateway, appsCfg.Jellyfin); err != nil {
					return fmt.Errorf("deploy jellyfin: %w", err)
				}
			}
		}
	}

	return nil
}
