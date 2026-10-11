package core

import (
	"cmp"
	"errors"
	"fmt"
	"strings"

	helmv3 "github.com/pulumi/pulumi-kubernetes/sdk/v4/go/kubernetes/helm/v3"
	"github.com/pulumi/pulumi/sdk/v3/go/pulumi"

	"github.com/kartikvashistha/homelab/pulumi/components/k8s/versions"
)

const (
	defaultCoreDNSReplicas           = 2
	defaultCoreDNSUpstream           = "1.1.1.1"
	metalLBLoadBalancerIPsAnnotation = "metallb.io/loadBalancerIPs"
	defaultCoreDNSNamespace          = "coredns"
)

// CoreDNSForwardZone forwards one zone (for example the router's own
// "home.arpa") to specific upstream resolvers.
type CoreDNSForwardZone struct {
	Zone      string   `json:"zone"`
	Upstreams []string `json:"upstreams"`
}

// CoreDNSConfig is the stack-config shape for `k8s:coredns`.
type CoreDNSConfig struct {
	// Namespace defaults to the coredns.
	Namespace string `json:"namespace,omitempty"`
	// Version overrides the pinned chart version in the versions package.
	Version string `json:"version,omitempty"`
	// LoadBalancerIP is the address MetalLB assigns to the DNS service; this
	// is the IP clients (or your router's DHCP) use as their DNS server.
	LoadBalancerIP string `json:"loadBalancerIp"`
	// Replicas defaults to 2.
	Replicas int `json:"replicas,omitempty"`
	// ForwardZones are zones forwarded to specific resolvers.
	ForwardZones []CoreDNSForwardZone `json:"forwardZones,omitempty"`
	// Upstreams resolve everything else. Defaults to 1.1.1.1.
	Upstreams []string `json:"upstreams,omitempty"`
}

type CoreDNSArgs struct {
	CoreDNSConfig

	// Domain is the private zone served by the gateway (e.g. "internal").
	// Every name under it resolves to GatewayIP, so new routes need no DNS
	// change.
	Domain string
	// GatewayIP is the gateway's LoadBalancer address.
	GatewayIP string
}

func (a *CoreDNSArgs) Validate() error {
	if !isIPv4(a.LoadBalancerIP) {
		return fmt.Errorf("loadBalancerIp %q is not a valid IPv4 address", a.LoadBalancerIP)
	}
	if !isIPv4(a.GatewayIP) {
		return fmt.Errorf("gateway IP %q is not a valid IPv4 address", a.GatewayIP)
	}
	if a.Domain == "" {
		return errors.New("gateway domain is required")
	}
	if a.Replicas < 0 {
		return fmt.Errorf("replicas must not be negative, got %d", a.Replicas)
	}
	for _, z := range a.ForwardZones {
		if z.Zone == "" {
			return errors.New("forwardZones entries need a zone")
		}
		if len(z.Upstreams) == 0 {
			return fmt.Errorf("forward zone %q needs at least one upstream", z.Zone)
		}
	}
	return nil
}

// CoreDNSComponent runs a small CoreDNS instance that serves the homelab:
//
//   - <Domain>:      every name resolves to the gateway (template plugin)
//   - ForwardZones:  forwarded to the configured resolvers
//   - everything else: forwarded to Upstreams, with caching
type CoreDNSComponent struct {
	pulumi.ResourceState
}

func NewCoreDNSComponent(
	ctx *pulumi.Context,
	name string,
	args *CoreDNSArgs,
	opts ...pulumi.ResourceOption,
) (*CoreDNSComponent, error) {
	if err := args.Validate(); err != nil {
		return nil, fmt.Errorf("coredns %q: %w", name, err)
	}

	upstreams := args.Upstreams
	if len(upstreams) == 0 {
		upstreams = []string{defaultCoreDNSUpstream}
	}

	servers := pulumi.Array{
		// <Domain> -> gateway. AAAA gets an empty NOERROR answer so clients
		// don't stall waiting on a record that doesn't exist.
		corednsServer(args.Domain,
			corednsPlugin("errors", "", ""),
			corednsPlugin("template", "IN A "+args.Domain,
				fmt.Sprintf(`answer "{{ .Name }} 60 IN A %s"`, args.GatewayIP)),
			corednsPlugin("template", "IN AAAA "+args.Domain, "rcode NOERROR"),
		),
	}

	for _, z := range args.ForwardZones {
		servers = append(servers, corednsServer(z.Zone,
			corednsPlugin("log", "", ""),
			corednsPlugin("forward", ". "+strings.Join(z.Upstreams, " "), ""),
			corednsPlugin("errors", "", ""),
		))
	}

	servers = append(servers, corednsServer(".",
		corednsPlugin("log", "", ""),
		corednsPlugin("forward", ". "+strings.Join(upstreams, " "), ""),
		corednsPlugin("errors", "", ""),
		// /health on :8080, required for the liveness probe
		corednsPlugin("health", "", "lameduck 5s"),
		// /ready on :8181, required for the readiness probe
		corednsPlugin("ready", "", ""),
		// /metrics on :9153
		corednsPlugin("prometheus", "0.0.0.0:9153", ""),
		corednsPlugin("cache", "30", ""),
		corednsPlugin("loop", "", ""),
		corednsPlugin("reload", "", ""),
		corednsPlugin("loadbalance", "", ""),
	))

	values := pulumi.Map{
		"serviceType":  pulumi.String("LoadBalancer"),
		"replicaCount": pulumi.Int(cmp.Or(args.Replicas, defaultCoreDNSReplicas)),
		"service": pulumi.Map{
			"annotations": pulumi.Map{
				metalLBLoadBalancerIPsAnnotation: pulumi.String(args.LoadBalancerIP),
			},
		},
		// Allow scheduling on control-plane nodes (single/small clusters).
		"tolerations": pulumi.Array{
			pulumi.Map{
				"key":      pulumi.String("node-role.kubernetes.io/control-plane"),
				"operator": pulumi.String("Exists"),
				"effect":   pulumi.String("NoSchedule"),
			},
		},
		"prometheus": pulumi.Map{
			"service": pulumi.Map{
				"enabled": pulumi.Bool(true),
				"annotations": pulumi.Map{
					"prometheus.io/scrape": pulumi.String("true"),
					"prometheus.io/port":   pulumi.String("9153"),
				},
			},
		},
		"servers": servers,
	}

	comp := &CoreDNSComponent{}
	if err := ctx.RegisterComponentResource("homelab:k8s:CoreDNS", name, comp, opts...); err != nil {
		return nil, fmt.Errorf("register coredns component: %w", err)
	}

	if _, err := helmv3.NewRelease(ctx, name+"-release", &helmv3.ReleaseArgs{
		Name:            pulumi.String(name),
		Chart:           pulumi.String("coredns"),
		Version:         pulumi.String(cmp.Or(args.Version, versions.CoreDNS)),
		Namespace:       pulumi.String(cmp.Or(args.Namespace, defaultCoreDNSNamespace)),
		CreateNamespace: pulumi.Bool(true),
		RepositoryOpts:  &helmv3.RepositoryOptsArgs{Repo: pulumi.String(versions.CoreDNSRepo)},
		Values:          values,
	}, pulumi.Parent(comp)); err != nil {
		return nil, fmt.Errorf("install coredns: %w", err)
	}

	if err := ctx.RegisterResourceOutputs(comp, pulumi.Map{
		"loadBalancerIp": pulumi.String(args.LoadBalancerIP),
	}); err != nil {
		return nil, err
	}
	return comp, nil
}

// corednsPlugin builds one entry of the chart's `servers[].plugins` list.
func corednsPlugin(name, parameters, configBlock string) pulumi.Map {
	p := pulumi.Map{"name": pulumi.String(name)}
	if parameters != "" {
		p["parameters"] = pulumi.String(parameters)
	}
	if configBlock != "" {
		p["configBlock"] = pulumi.String(configBlock)
	}
	return p
}

// corednsServer builds one entry of the chart's `servers` list: a server
// block for a single zone, listening on port 53 over UDP and TCP.
func corednsServer(zone string, plugins ...pulumi.Map) pulumi.Map {
	list := make(pulumi.Array, len(plugins))
	for i, p := range plugins {
		list[i] = p
	}
	return pulumi.Map{
		"zones": pulumi.Array{
			pulumi.Map{"zone": pulumi.String(zone), "use_tcp": pulumi.Bool(true)},
		},
		"port":    pulumi.Int(53),
		"plugins": list,
	}
}
