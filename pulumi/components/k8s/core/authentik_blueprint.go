package core

import (
	"bytes"
	"text/template"
)

// blueprintTmpl renders one OAuth2 provider + application pair per client.
// Free-text fields go through printf "%q" so they're emitted as quoted YAML
// scalars; slugs are validated against clientSlugRe, so they're safe bare.
//
// grant_types must be explicit: on authentik 2026.x a provider created from a
// blueprint without it rejects every authorize request ("Invalid grant_type").
//
// signing_key matters: without it Authentik signs ID tokens with HS256 using
// the client secret, and consumers like the kube-apiserver need RS256.
const blueprintTmpl = `# Managed by Pulumi. Changes made in the Authentik UI will be overwritten.
version: 1
metadata:
  name: homelab-oidc
entries:
{{- range .}}
  - model: authentik_providers_oauth2.oauth2provider
    id: {{.Slug}}-provider
    identifiers:
      name: {{.Slug}}
    attrs:
      client_type: confidential
      grant_types:
        - authorization_code
        - refresh_token
      client_id: {{.Slug}}
      client_secret: !Env {{.EnvVar}}
      authorization_flow: !Find [authentik_flows.flow, [slug, default-provider-authorization-implicit-consent]]
      invalidation_flow: !Find [authentik_flows.flow, [slug, default-provider-invalidation-flow]]
      signing_key: !Find [authentik_crypto.certificatekeypair, [name, authentik Self-signed Certificate]]
      redirect_uris:
{{- range .RedirectURIs}}
        - matching_mode: strict
          url: {{printf "%q" .}}
{{- end}}
      property_mappings:
        - !Find [authentik_providers_oauth2.scopemapping, [managed, goauthentik.io/providers/oauth2/scope-openid]]
        - !Find [authentik_providers_oauth2.scopemapping, [managed, goauthentik.io/providers/oauth2/scope-email]]
        - !Find [authentik_providers_oauth2.scopemapping, [managed, goauthentik.io/providers/oauth2/scope-profile]]
  - model: authentik_core.application
    identifiers:
      slug: {{.Slug}}
    attrs:
      name: {{printf "%q" .Name}}
      provider: !KeyOf {{.Slug}}-provider
{{- if .LaunchURL}}
      meta_launch_url: {{printf "%q" .LaunchURL}}
{{- end}}
{{- end}}
`

var blueprintTemplate = template.Must(template.New("blueprint").Parse(blueprintTmpl))

func renderBlueprint(clients []OIDCClient) (string, error) {
	var buf bytes.Buffer
	if err := blueprintTemplate.Execute(&buf, clients); err != nil {
		return "", err
	}
	return buf.String(), nil
}
