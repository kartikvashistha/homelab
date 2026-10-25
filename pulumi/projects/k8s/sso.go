package main

import (
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
)

// ssoHelmValues returns the OIDC values for a Helm release, or nil when the
// release has no SSO wiring or no client of the same slug is configured under
// k8s:sso.
//
// This is a switch rather than config because each chart spells its OIDC
// values differently. To wire another chart, add a case here and a matching
// client in k8s:sso; the release name is the client slug.
func ssoHelmValues(sso *core.AuthentikComponent, release string) (pulumi.Map, error) {
	if !sso.HasClient(release) {
		return nil, nil
	}

	secret, err := sso.ClientSecret(release)
	if err != nil {
		return nil, err
	}

	switch release {
	case "headlamp":
		// Headlamp's chart creates its own Secret from these values.
		// Note the kube-apiserver must also trust Authentik for Headlamp to
		// act as the logged-in user (--oidc-issuer-url, --oidc-client-id).
		return pulumi.Map{
			"config": pulumi.Map{
				"oidc": pulumi.Map{
					"clientID":     pulumi.String(release),
					"clientSecret": secret,
					"issuerURL":    pulumi.String(sso.IssuerURL(release)),
					"scopes":       pulumi.String("openid profile email"),
				},
			},
		}, nil
	}

	return nil, nil
}
