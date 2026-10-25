// Package versions is the single place where chart and CRD versions pinned in
// code live.
//
// Each pin is preceded by a "renovate:" annotation; the regex manager in
// renovate.json reads it, so Renovate opens PRs to bump these. Keep the
// annotation on the line directly above the constant it describes.
//
// Istio's base, istiod, cni and ztunnel charts are released in lock-step, so one
// constant covers all four.
package versions

const (
	CiliumRepo = "https://helm.cilium.io"
	// renovate: datasource=helm depName=cilium registryUrl=https://helm.cilium.io
	Cilium = "1.20.4"

	// renovate: datasource=github-releases depName=kubernetes-sigs/gateway-api
	GatewayAPI = "v1.6.0"

	MetalLBRepo = "https://metallb.github.io/metallb"
	// renovate: datasource=helm depName=metallb registryUrl=https://metallb.github.io/metallb
	MetalLB = "0.16.1"

	IstioRepo = "https://blob.istio.io/istio-release/charts"
	// renovate: datasource=helm depName=istiod registryUrl=https://blob.istio.io/istio-release/charts
	Istio = "1.31.0"

	CertManagerRepo = "https://charts.jetstack.io"
	// renovate: datasource=helm depName=cert-manager registryUrl=https://charts.jetstack.io
	CertManager = "1.21.2"

	LonghornRepo = "https://charts.longhorn.io"
	// renovate: datasource=helm depName=longhorn registryUrl=https://charts.longhorn.io
	Longhorn = "1.12.1"

	CoreDNSRepo = "https://coredns.github.io/helm"
	// renovate: datasource=helm depName=coredns registryUrl=https://coredns.github.io/helm
	CoreDNS = "1.47.1"

	KialiOperatorRepo = "https://kiali.org/helm-charts"
	// renovate: datasource=helm depName=kiali-operator registryUrl=https://kiali.org/helm-charts
	KialiOperator = "2.32.0"
)
