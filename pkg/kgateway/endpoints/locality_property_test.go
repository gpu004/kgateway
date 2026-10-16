package endpoints

import (
	"os"
	"slices"
	"testing"

	envoycorev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	"hegel.dev/go/hegel"
	v1alpha3 "istio.io/api/networking/v1alpha3"
	corev1 "k8s.io/api/core/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func drawPropertyLocality(ht *hegel.T) *envoycorev3.Locality {
	regions := []string{"r1", "r2", "r3"}
	zones := []string{"z1", "z2", "z3"}
	subzones := []string{"s1", "s2"}
	return &envoycorev3.Locality{
		Region:  regions[hegel.Draw(ht, hegel.Integers(0, len(regions)-1))],
		Zone:    zones[hegel.Draw(ht, hegel.Integers(0, len(zones)-1))],
		SubZone: subzones[hegel.Draw(ht, hegel.Integers(0, len(subzones)-1))],
	}
}

func drawPropertyLabel(ht *hegel.T) string {
	values := []string{"", "a", "b", "r1", "r2", "z1", "z2"}
	return values[hegel.Draw(ht, hegel.Integers(0, len(values)-1))]
}

func TestApplyLocalityFailoverProperties(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		count := hegel.Draw(ht, hegel.Integers(1, 6))
		proxy := drawPropertyLocality(ht)
		cla := &envoyendpointv3.ClusterLoadAssignment{}
		preFailoverKeys := make([]int, count)
		for i := range count {
			locality := drawPropertyLocality(ht)
			priority := uint32(hegel.Draw(ht, hegel.Integers(0, 2)))
			preFailoverKeys[i] = int(priority)*5 + LbPriority(proxy, locality)
			cla.Endpoints = append(cla.Endpoints, &envoyendpointv3.LocalityLbEndpoints{
				Locality: locality,
				Priority: priority,
			})
		}
		var failover []*v1alpha3.LocalityLoadBalancerSetting_Failover
		if hegel.Draw(ht, hegel.Integers(0, 1)) == 1 {
			regions := []string{"r1", "r2", "r3"}
			failover = append(failover, &v1alpha3.LocalityLoadBalancerSetting_Failover{
				From: proxy.Region,
				To:   regions[hegel.Draw(ht, hegel.Integers(0, len(regions)-1))],
			})
		}
		applyLocalityFailover(proxy, cla, failover)

		gotPriorities := make(map[uint32]struct{}, count)
		for _, endpoint := range cla.GetEndpoints() {
			gotPriorities[endpoint.GetPriority()] = struct{}{}
		}
		for priority := range len(gotPriorities) {
			if _, found := gotPriorities[uint32(priority)]; !found {
				ht.Fatalf("priorities are not contiguous from zero: got %v", gotPriorities)
			}
		}
		for i := range count {
			for j := range count {
				if preFailoverKeys[i] < preFailoverKeys[j] &&
					cla.Endpoints[i].GetPriority() > cla.Endpoints[j].GetPriority() {
					ht.Fatalf("pre-failover order changed: keys=%v priorities=%v", preFailoverKeys,
						propertyEndpointPriorities(cla.GetEndpoints()))
				}
			}
		}
	}, hegel.WithTestCases(120))
}

func propertyEndpointPriorities(endpoints []*envoyendpointv3.LocalityLbEndpoints) []uint32 {
	priorities := make([]uint32, len(endpoints))
	for i, endpoint := range endpoints {
		priorities[i] = endpoint.GetPriority()
	}
	return priorities
}

func TestPrioritizeEndpointsTrafficDistributionPrioritiesAreContiguous(t *testing.T) {
	if os.Getenv("KGATEWAY_HEGEL_KNOWN_BUGS") == "" {
		t.Skip("known bug: traffic-distribution priorities can be sparse when no endpoint matches higher-ranked labels")
	}
	hegel.Test(t, func(ht *hegel.T) {
		count := hegel.Draw(ht, hegel.Integers(1, 6))
		localities := []ir.PodLocality{
			{Region: "r1", Zone: "z1", Subzone: "s1"},
			{Region: "r1", Zone: "z2", Subzone: "s2"},
			{Region: "r2", Zone: "z1", Subzone: "s1"},
			{Region: "r2", Zone: "z3", Subzone: "s2"},
		}
		endpoints := ir.NewEndpointsForBackend(ir.NewBackendObjectIR(ir.ObjectSource{
			Group: "core", Kind: "Service", Namespace: "ns", Name: "svc",
		}, 80, "", ""))
		for i := range count {
			locality := localities[hegel.Draw(ht, hegel.Integers(0, len(localities)-1))]
			endpoint := prioritizeTestEndpoint(locality, i)
			endpoint.EndpointMd.Labels = map[string]string{
				corev1.LabelTopologyRegion:  drawPropertyLabel(ht),
				corev1.LabelTopologyZone:    drawPropertyLabel(ht),
				"topology.istio.io/network": drawPropertyLabel(ht),
				"topology.istio.io/subzone": drawPropertyLabel(ht),
				corev1.LabelHostname:        drawPropertyLabel(ht),
			}
			endpoints.Add(locality, endpoint)
		}
		distributions := []wellknown.TrafficDistribution{
			wellknown.TrafficDistributionPreferSameZone,
			wellknown.TrafficDistributionPreferSameNode,
		}
		distribution := distributions[hegel.Draw(ht, hegel.Integers(0, len(distributions)-1))]
		priorityInfo := priorityInfoFromTrafficDistribution(distribution)
		proxyLabels := map[string]string{
			corev1.LabelTopologyRegion:  drawPropertyLabel(ht),
			corev1.LabelTopologyZone:    drawPropertyLabel(ht),
			"topology.istio.io/network": drawPropertyLabel(ht),
			"topology.istio.io/subzone": drawPropertyLabel(ht),
			corev1.LabelHostname:        drawPropertyLabel(ht),
		}
		proxy := ir.NewUniquelyConnectedClient("role", "ns", proxyLabels, ir.PodLocality{})
		cla := PrioritizeEndpoints(nil, proxy, EndpointsInputs{
			EndpointsForBackend: *endpoints,
			PriorityInfo:        priorityInfo,
		})
		priorities := propertyEndpointPriorities(cla.GetEndpoints())
		unique := make(map[uint32]struct{}, len(priorities))
		for _, priority := range priorities {
			unique[priority] = struct{}{}
		}
		ordered := make([]uint32, 0, len(unique))
		for priority := range unique {
			ordered = append(ordered, priority)
		}
		slices.Sort(ordered)
		for i, priority := range ordered {
			if priority != uint32(i) {
				ht.Fatalf("traffic distribution %v emitted non-contiguous priorities: %v", distribution, priorities)
			}
		}
	}, hegel.WithTestCases(120))
}
