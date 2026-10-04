package apps

import (
	"fmt"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi/config"
)

type App interface {
	Name() string

	Deploy(
		ctx *pulumi.Context,
		cfg *config.Config,
		gateway *core.GatewayComponent,
		storage *core.StorageClassComponent,
	) error
}

var registry = map[string]App{
	"jellyfin": JellyfinApp{},

	// Future applications:
	// "headlamp": HeadlampApp{},
	// "kiali":    KialiApp{},
}

// Deploy discovers applications from the k8s:apps configuration.
//
// The Pulumi configuration is structured like:
//
//	k8s:apps:
//	  jellyfin:
//	    address:
//	      - 192.168.50.89
//	    targetPort: 8899
//	    hostnames:
//	      - jellyfin.internal
//
// Therefore we must read the "apps" object. Using:
//
//	cfg.Get("apps:jellyfin")
//
// does not correctly detect an object-valued nested configuration.
func Deploy(
	ctx *pulumi.Context,
	cfg *config.Config,
	gateway *core.GatewayComponent,
	storage *core.StorageClassComponent,
) error {
	var configuredApps map[string]map[string]any

	if err := cfg.GetObject(
		"apps",
		&configuredApps,
	); err != nil {
		return fmt.Errorf(
			"invalid k8s:apps configuration: %w",
			err,
		)
	}

	for name, app := range registry {
		if _, ok := configuredApps[name]; !ok {
			continue
		}

		if err := app.Deploy(
			ctx,
			cfg,
			gateway,
			storage,
		); err != nil {
			return fmt.Errorf(
				"deploy %q: %w",
				name,
				err,
			)
		}
	}

	return nil
}
