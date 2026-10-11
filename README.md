# homelab

Pulumi (Go) code for my homelab Kubernetes cluster(s).

## Layout

```
go.mod                          single Go module for everything
renovate.json                   keeps chart/CRD pins current (see Versions)
pulumi/
  components/k8s/
    versions/                   every chart/CRD version pinned in code
    core/
      platform.go               Networking -> CertManager -> Gateway (+ Longhorn)
      networking.go metallb.go istio.go certmanager.go longhorn.go gateway.go
      coredns.go                homelab DNS; *.<domain> -> gateway
      helmrelease.go            Helm chart + optional gateway route
      externalservice.go        LAN host fronted by the gateway
      httproute.go              building block
    apps/                       opt-in in-cluster apps, selected via `k8s:apps`
  projects/k8s/
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
just fmt vet lint
```

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
