package apps

import (
	"cmp"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	corev1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/core/v1"
	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	metav1 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/meta/v1"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/core"
	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

// observabilityFiles holds the base Helm values for each chart plus the Alloy
// pipeline. They are plain YAML/Alloy files so they read (and diff) like the
// real thing instead of as nested Go maps.
//
//go:embed observability/*.yaml observability/config.alloy
var observabilityFiles embed.FS

const (
	obsDefaultNamespace      = "observability"
	obsDefaultGrafanaStorage = "5Gi"
	obsDefaultLokiStorage    = "20Gi"
	obsDefaultLokiRetention  = "336h" // 14d
	obsDefaultTempoStorage   = "10Gi"
	obsDefaultTempoRetention = "168h" // 7d
	obsDefaultMimirStorage   = "20Gi"
	obsDefaultMimirRetention = "30d"

	// The Grafana chart's Service is named after the release and listens on 80.
	obsGrafanaService = "grafana"
	obsGrafanaPort    = 80

	// Mimir's first install pulls and starts ~10 workloads; Helm's default
	// 5 minute wait is too tight on a homelab.
	obsMimirTimeoutSeconds = 900
)

var (
	obsNamespaceRE = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]*[a-z0-9])?$`)
	// Cluster names end up inside Alloy string literals and metric labels.
	obsClusterRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.-]*$`)
)

// ObservabilityArgs is the `k8s:apps.observability` stack config. Everything
// except hostnames is optional.
type ObservabilityArgs struct {
	// Hostnames route to the Grafana UI through the cluster gateway.
	Hostnames []string `json:"hostnames"`
	// Namespace for the whole stack. Defaults to "observability".
	Namespace string `json:"namespace,omitempty"`
	// ClusterName becomes the `cluster` label on all collected telemetry.
	// Defaults to the Pulumi stack name.
	ClusterName string `json:"clusterName,omitempty"`

	Grafana obsGrafanaArgs `json:"grafana"`
	Loki    obsLokiArgs    `json:"loki"`
	Tempo   obsTempoArgs   `json:"tempo"`
	Mimir   obsMimirArgs   `json:"mimir"`
}

type obsGrafanaArgs struct {
	StorageSize string `json:"storageSize,omitempty"`
}

type obsLokiArgs struct {
	StorageSize string `json:"storageSize,omitempty"`
	Retention   string `json:"retention,omitempty"`
}

type obsTempoArgs struct {
	StorageSize string `json:"storageSize,omitempty"`
	Retention   string `json:"retention,omitempty"`
}

type obsMimirArgs struct {
	// StorageSize sizes the bundled object store holding Mimir's blocks.
	StorageSize string `json:"storageSize,omitempty"`
	// Retention is how long metric blocks are kept (e.g. "30d").
	Retention string `json:"retention,omitempty"`
}

// obsSettings is ObservabilityArgs with defaults applied and validated.
type obsSettings struct {
	namespace string
	cluster   string
	hostnames []string

	grafanaStorage string
	lokiStorage    string
	lokiRetention  string
	tempoStorage   string
	tempoRetention string
	mimirStorage   string
	mimirRetention string
}

func (a *ObservabilityArgs) settings(stack string) (*obsSettings, error) {
	s := &obsSettings{
		namespace:      cmp.Or(a.Namespace, obsDefaultNamespace),
		cluster:        cmp.Or(a.ClusterName, stack),
		hostnames:      a.Hostnames,
		grafanaStorage: cmp.Or(a.Grafana.StorageSize, obsDefaultGrafanaStorage),
		lokiStorage:    cmp.Or(a.Loki.StorageSize, obsDefaultLokiStorage),
		lokiRetention:  cmp.Or(a.Loki.Retention, obsDefaultLokiRetention),
		tempoStorage:   cmp.Or(a.Tempo.StorageSize, obsDefaultTempoStorage),
		tempoRetention: cmp.Or(a.Tempo.Retention, obsDefaultTempoRetention),
		mimirStorage:   cmp.Or(a.Mimir.StorageSize, obsDefaultMimirStorage),
		mimirRetention: cmp.Or(a.Mimir.Retention, obsDefaultMimirRetention),
	}

	if len(s.hostnames) == 0 {
		return nil, errors.New("observability requires at least one hostname (for Grafana)")
	}
	if !obsNamespaceRE.MatchString(s.namespace) {
		return nil, fmt.Errorf("observability namespace %q is not a valid DNS label", s.namespace)
	}
	if !obsClusterRE.MatchString(s.cluster) {
		return nil, fmt.Errorf("observability clusterName %q must match %s", s.cluster, obsClusterRE)
	}
	return s, nil
}

type ObservabilityComponent struct {
	pulumi.ResourceState
}

// NewObservabilityComponent installs the LGTM stack and the collector that
// feeds it:
//
//	Grafana  <- Mimir (metrics), Loki (logs), Tempo (traces)
//	Alloy    -> scrapes the cluster, tails logs, receives OTLP, writes to the three
//	kube-state-metrics, node-exporter, Prometheus Operator CRDs (CRDs only)
//
// Grafana is routed through the cluster gateway; the backends are ClusterIP
// only.
func NewObservabilityComponent(
	ctx *pulumi.Context,
	name string,
	args *ObservabilityArgs,
	env Env,
	opts ...pulumi.ResourceOption,
) (*ObservabilityComponent, error) {
	s, err := args.settings(ctx.Stack())
	if err != nil {
		return nil, err
	}
	if env.Gateway == nil {
		return nil, errors.New("observability requires the cluster gateway")
	}

	comp := &ObservabilityComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:Observability", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register observability component: %w", err)
	}
	parent := pulumi.Parent(comp)

	ns, err := corev1.NewNamespace(ctx, name+"-ns", &corev1.NamespaceArgs{
		Metadata: &metav1.ObjectMetaArgs{Name: pulumi.String(s.namespace)},
	}, parent)
	if err != nil {
		return nil, fmt.Errorf("create observability namespace: %w", err)
	}

	in := &obsInstaller{ctx: ctx, name: name, env: env, ns: ns, parent: parent}

	// CRDs only: lets anything that ships a ServiceMonitor/PodMonitor/
	// PrometheusRule apply cleanly. No Prometheus Operator runs; Alloy reads
	// these objects.
	crds, err := in.install(obsChart{
		release: "prometheus-operator-crds",
		chart:   "prometheus-operator-crds",
		repo:    versions.PrometheusCommunityRepo,
		version: versions.PrometheusOperatorCRDs,
	})
	if err != nil {
		return nil, err
	}

	mimir, err := in.install(obsChart{
		release: "mimir",
		chart:   "mimir-distributed",
		repo:    versions.GrafanaLabsRepo,
		version: versions.Mimir,
		values:  "mimir.yaml",
		timeout: obsMimirTimeoutSeconds,
		subst: map[string]string{
			"RETENTION":    s.mimirRetention,
			"STORAGE_SIZE": s.mimirStorage,
		},
	})
	if err != nil {
		return nil, err
	}

	loki, err := in.install(obsChart{
		release: "loki",
		chart:   "loki",
		repo:    versions.GrafanaCommunityRepo,
		version: versions.Loki,
		values:  "loki.yaml",
		subst: map[string]string{
			"RETENTION":    s.lokiRetention,
			"STORAGE_SIZE": s.lokiStorage,
		},
	})
	if err != nil {
		return nil, err
	}

	tempo, err := in.install(obsChart{
		release: "tempo",
		chart:   "tempo",
		repo:    versions.GrafanaCommunityRepo,
		version: versions.Tempo,
		values:  "tempo.yaml",
		subst: map[string]string{
			"RETENTION":    s.tempoRetention,
			"STORAGE_SIZE": s.tempoStorage,
		},
	})
	if err != nil {
		return nil, err
	}

	grafana, err := in.install(obsChart{
		release: "grafana",
		chart:   "grafana",
		repo:    versions.GrafanaCommunityRepo,
		version: versions.Grafana,
		values:  "grafana.yaml",
		subst: map[string]string{
			"NAMESPACE":    s.namespace,
			"HOSTNAME":     s.hostnames[0],
			"STORAGE_SIZE": s.grafanaStorage,
		},
	})
	if err != nil {
		return nil, err
	}

	ksm, err := in.install(obsChart{
		release: "kube-state-metrics",
		chart:   "kube-state-metrics",
		repo:    versions.PrometheusCommunityRepo,
		version: versions.KubeStateMetrics,
	})
	if err != nil {
		return nil, err
	}

	nodeExporter, err := in.install(obsChart{
		release: "prometheus-node-exporter",
		chart:   "prometheus-node-exporter",
		repo:    versions.PrometheusCommunityRepo,
		version: versions.PrometheusNodeExporter,
	})
	if err != nil {
		return nil, err
	}

	alloyConfig, err := observabilityRender("config.alloy", map[string]string{
		"NAMESPACE": s.namespace,
		"CLUSTER":   s.cluster,
	})
	if err != nil {
		return nil, err
	}

	// Alloy goes last so its destinations and the CRDs it watches exist.
	if _, err := in.install(obsChart{
		release: "alloy",
		chart:   "alloy",
		repo:    versions.GrafanaLabsRepo,
		version: versions.Alloy,
		values:  "alloy.yaml",
		subst:   map[string]string{"ALLOY_CONFIG": obsIndent(alloyConfig, 6)},
	}, crds, mimir, loki, tempo, ksm, nodeExporter); err != nil {
		return nil, err
	}

	if _, err := core.NewHTTPRouteComponent(ctx, name+"-grafana-route", &core.HTTPRouteArgs{
		Name:      "grafana",
		Namespace: s.namespace,
		Hostnames: s.hostnames,
		Gateway:   env.Gateway,
		Service:   obsGrafanaService,
		Port:      obsGrafanaPort,
	}, parent, pulumi.DependsOn([]pulumi.Resource{env.Gateway, grafana})); err != nil {
		return nil, fmt.Errorf("create grafana route: %w", err)
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{}); err != nil {
		return nil, err
	}
	return comp, nil
}

// obsChart describes one Helm release of the stack.
type obsChart struct {
	// release is the Helm release name. It also names the per-cluster override
	// directory (helm-overrides/<release>/) and, for most charts, their Service.
	release string
	chart   string
	repo    string
	version string
	// values is the embedded base values file ("" for chart defaults).
	values string
	// subst fills @@KEY@@ placeholders in values.
	subst map[string]string
	// timeout is the Helm wait in seconds (0: provider default).
	timeout int
}

type obsInstaller struct {
	ctx    *pulumi.Context
	name   string
	env    Env
	ns     *corev1.Namespace
	parent pulumi.ResourceOption
}

// install creates one Helm release. Values files are applied in order: the
// embedded base, then the cluster's override file (if any), so a cluster can
// change anything without touching code.
func (in *obsInstaller) install(c obsChart, deps ...pulumi.Resource) (*helmv3.Release, error) {
	var files pulumi.AssetOrArchiveArray

	if c.values != "" {
		base, err := observabilityRender(c.values, c.subst)
		if err != nil {
			return nil, fmt.Errorf("render %s values: %w", c.release, err)
		}
		files = append(files, pulumi.NewStringAsset(base))
	}

	if in.env.HelmValueFiles != nil {
		overrides, err := in.env.HelmValueFiles(c.release)
		if err != nil {
			return nil, err
		}
		for _, f := range overrides {
			files = append(files, pulumi.NewFileAsset(f))
		}
	}

	rel := &helmv3.ReleaseArgs{
		Name:           pulumi.String(c.release),
		Chart:          pulumi.String(c.chart),
		Version:        pulumi.String(c.version),
		Namespace:      in.ns.Metadata.Name(),
		RepositoryOpts: &helmv3.RepositoryOptsArgs{Repo: pulumi.String(c.repo)},
		ValueYamlFiles: files,
	}
	if c.timeout > 0 {
		rel.Timeout = pulumi.Int(c.timeout)
	}

	opts := []pulumi.ResourceOption{in.parent}
	if len(deps) > 0 {
		opts = append(opts, pulumi.DependsOn(deps))
	}

	release, err := helmv3.NewRelease(in.ctx, in.name+"-"+c.release, rel, opts...)
	if err != nil {
		return nil, fmt.Errorf("install %s: %w", c.release, err)
	}
	return release, nil
}

// observabilityRender reads an embedded file and fills its @@KEY@@
// placeholders. A placeholder left over is an error, so a typo fails in
// preview instead of shipping literally into a chart.
func observabilityRender(file string, subst map[string]string) (string, error) {
	raw, err := observabilityFiles.ReadFile("observability/" + file)
	if err != nil {
		return "", fmt.Errorf("read embedded %s: %w", file, err)
	}

	pairs := make([]string, 0, 2*len(subst))
	for k, v := range subst {
		pairs = append(pairs, "@@"+k+"@@", v)
	}
	out := strings.NewReplacer(pairs...).Replace(string(raw))

	if i := strings.Index(out, "@@"); i >= 0 {
		return "", fmt.Errorf("%s: unreplaced placeholder near %q", file, out[i:min(len(out), i+30)])
	}
	return out, nil
}

// obsIndent indents every non-empty line by n spaces, for embedding a block
// into a YAML literal scalar.
func obsIndent(s string, n int) string {
	pad := strings.Repeat(" ", n)
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i, l := range lines {
		if l != "" {
			lines[i] = pad + l
		}
	}
	return strings.Join(lines, "\n")
}

type ObservabilityApp struct{}

func (ObservabilityApp) Name() string { return "observability" }

func (ObservabilityApp) Deploy(ctx *pulumi.Context, env Env, raw json.RawMessage) error {
	args, err := decode[ObservabilityArgs](raw)
	if err != nil {
		return fmt.Errorf("decode observability config: %w", err)
	}

	if _, err := NewObservabilityComponent(ctx, "observability", &args, env); err != nil {
		return fmt.Errorf("create observability component: %w", err)
	}
	return nil
}
