# homelab

Pulumi (Go) code for my homelab Kubernetes cluster(s).

They call me (a failed) Victor Frankenstein when I work in this repo.

## Layout

```
go.work                         ties the two modules together locally
renovate.json                   keeps chart/CRD pins current (see Versions)
pulumi/
  components/k8s/               module: .../pulumi/components/k8s
    versions/                   every chart/CRD version pinned in code
    core/
      platform.go               Networking -> CertManager -> Gateway (+ Longhorn)
      networking.go metallb.go istio.go certmanager.go longhorn.go gateway.go
      coredns.go                homelab DNS; *.<domain> -> gateway
      helmrelease.go            Helm chart + optional gateway route
      externalservice.go        LAN host fronted by the gateway
      httproute.go              building block
    apps/                       opt-in in-cluster apps, selected via `k8s:apps`
  projects/k8s/                 module: .../pulumi/projects/k8s (own go.mod,
                                  next to Pulumi.yaml, as `pulumi package add`
                                  expects)
    main.go                     wires config -> components, in order
    config.go                   typed stack-config loading
    cluster.go                  clusters/<cluster>/ layout knowledge
    Pulumi.<stack>.yaml         per-cluster config (one stack per cluster)
    clusters/<cluster>/
      helm-overrides/<release>/values.yaml
      manifests/*.yaml          raw manifests applied as-is
```

One stack per cluster. `kubernetes:context` picks the kube context; the
`clusters/<name>/` directory defaults to the context name (override with
`k8s:cluster`).

Istio, cert-manager and the gateway are always installed (the gateway can't
exist without them). MetalLB (`k8s:networking`) and Longhorn (`k8s:storage`) are
opt-in.

## Common tasks

```
just preview     # pulumi preview --diff
just up
just fmt vet lint tidy    # vet/lint/tidy loop over both modules
```

There is no go.mod at the repo root, so run Go tooling from inside a module
(or via `just`); `go vet ./...` at the root has nothing to match.

## Typed CRDs (provider extensions)

Pulumi can generate typed SDKs for CRDs as an extension of the kubernetes
provider ([tutorial](https://www.pulumi.com/dev/tutorials/typed-customresources-with-provider-extensions/)).
Run it from the project directory so the SDK, the `Pulumi.yaml` entry and the
`go.mod` `replace` all land next to the project:

```
cd pulumi/projects/k8s
pulumi package add kubernetes --extension "name=gateway-networking crd-manifest=<path>.yaml"
```

Heads-up: the components module currently builds untyped `CustomResource`s
(Gateway, HTTPRoute, MetalLB, cert-manager). A generated SDK lives under the
*project's* `sdks/` directory, so using it from the components module means
making that SDK reachable from both modules (e.g. `use` it in `go.work` and
`replace` it in each `go.mod`), or moving the typed resources into the project.
Decide that when you adopt the first extension.

## Recipes

**Add a Helm chart, optionally exposed on a hostname** — add to `k8s:helm`:

```yaml
- releaseName: headlamp
  chart: headlamp
  namespace: headlamp
  repo: "https://kubernetes-sigs.github.io/headlamp"
  # renovate: datasource=helm depName=headlamp registryUrl=https://kubernetes-sigs.github.io/headlamp
  version: 0.45.0
  route:               # optional
    hostnames: [headlamp.internal]
    port: 80           # service defaults to releaseName
```

Per-cluster values go in `clusters/<cluster>/helm-overrides/<releaseName>/values.yaml`.

**Expose a LAN service (NAS, media server, ...)** — no code needed:

```yaml
k8s:externalServices:
  nas:
    addresses: [192.168.50.20]
    port: 5000
    hostnames: [nas.internal]
```

**DNS needs no change for either of the above.** CoreDNS answers every name
under the gateway domain (`k8s:gateway.domain`, default `internal`) with the
gateway IP. Point your router's DHCP (or clients) at `k8s:coredns.loadBalancerIp`.

**Add an in-cluster app** — implement `apps.App` in a new file, add it to
`registry` in `apps/apps.go`, enable it under `k8s:apps`. Unknown keys under
`k8s:apps` fail the preview.

## SSO (Authentik)

`k8s:sso.clients` lists the OIDC applications. Each becomes an Authentik OAuth2
provider + application, declared as a [blueprint](https://docs.goauthentik.io/customize/blueprints/)
in a ConfigMap. Secrets never appear in it: the blueprint reads them with
`!Env`, and Pulumi supplies them through a Secret loaded as env vars.

Secrets come from a Pulumi ESC environment (see
`pulumi/projects/k8s/esc/dreadnought.example.yaml`) that the stack attaches
with `environment:`. Needed config keys: `k8s:authentik-secret-key`,
`k8s:authentik-pg-password`, and one `k8s:oidc-secret-<slug>` per client.

**Add an in-cluster app** (e.g. a chart with OIDC support):

1. Add a client to `k8s:sso.clients` and a `oidc-secret-<slug>` in ESC.
2. For a Helm release, add a case to `ssoHelmValues` in
   `projects/k8s/sso.go` (the release name must equal the client slug). For an
   `apps.App`, use `deps.SSO.IssuerURL(slug)` / `deps.SSO.ClientSecret(slug)`.

**Jellyfin (outside the cluster)** has no native OIDC. Pulumi only registers the
Authentik side. Install the SSO plugin (`9p4/jellyfin-plugin-sso`) and add a
provider named `authentik` with:

```
OID Endpoint:  https://sso.internal/application/o/jellyfin/
Client ID:     jellyfin
Client Secret: esc env get homelab/dreadnought sso.clients.jellyfin --show-secrets
```

Things that bite:

- Pods must resolve and trust `sso.internal`. kube-dns doesn't know the
  homelab zone unless you forward it to `k8s:coredns.loadBalancerIp`, and the
  gateway cert is signed by the internal CA.
- Headlamp acts as the logged-in user, so the kube-apiserver needs
  `--oidc-issuer-url=https://sso.internal/application/o/headlamp/` and
  `--oidc-client-id=headlamp`, plus RBAC for your Authentik groups.
- Rotating a client secret changes the Secret only. Restart the Authentik
  server and worker pods so they pick up the new env var.
- The Postgres password in ESC must match the one the database was created
  with; changing it later does not change it inside an existing PVC.

## Verifying DNS after `just up`

```
dig @<coredns loadBalancerIp> anything.internal +short    # -> gateway IP
dig @<coredns loadBalancerIp> anything.internal AAAA      # -> NOERROR, empty answer
dig @<coredns loadBalancerIp> example.com +short          # -> forwarded upstream
```

## Versions

Chart/CRD versions pinned in code live in
`pulumi/components/k8s/versions/versions.go`; those set in stack config sit next
to a `# renovate:` comment. `renovate.json` has one regex manager that reads
both, so Renovate opens a PR per bump. Go dependencies are handled by Renovate's
default `gomod` manager.
