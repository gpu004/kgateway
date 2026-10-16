package routeutils

import (
	"testing"
	"time"

	"hegel.dev/go/hegel"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
)

func drawSortableRoute(ht *hegel.T) *SortableRoute {
	names := []string{"a", "b", "c"}
	namespaces := []string{"ns-a", "ns-b"}
	name := names[hegel.Draw(ht, hegel.Integers(0, len(names)-1))]
	namespace := namespaces[hegel.Draw(ht, hegel.Integers(0, len(namespaces)-1))]
	created := time.Unix(int64(hegel.Draw(ht, hegel.Integers(0, 2))), 0)
	pathTypes := []gwv1.PathMatchType{gwv1.PathMatchExact, gwv1.PathMatchPathPrefix, gwv1.PathMatchRegularExpression}
	pathType := pathTypes[hegel.Draw(ht, hegel.Integers(0, len(pathTypes)-1))]
	paths := []string{"/", "/a", "/a/b", "/x.*", "/same"}
	pathValue := paths[hegel.Draw(ht, hegel.Integers(0, len(paths)-1))]
	match := gwv1.HTTPRouteMatch{
		Path: &gwv1.HTTPPathMatch{Type: &pathType, Value: &pathValue},
	}
	if hegel.Draw(ht, hegel.Integers(0, 1)) == 1 {
		methods := []gwv1.HTTPMethod{gwv1.HTTPMethodGet, gwv1.HTTPMethodPost}
		method := methods[hegel.Draw(ht, hegel.Integers(0, len(methods)-1))]
		match.Method = &method
	}
	headerCount := hegel.Draw(ht, hegel.Integers(0, 3))
	for range headerCount {
		name := []gwv1.HTTPHeaderName{"x-a", "x-b", "x-c"}[hegel.Draw(ht, hegel.Integers(0, 2))]
		value := []string{"a", "b"}[hegel.Draw(ht, hegel.Integers(0, 1))]
		match.Headers = append(match.Headers, gwv1.HTTPHeaderMatch{
			Type:  new(gwv1.HeaderMatchExact),
			Name:  name,
			Value: value,
		})
	}
	queryCount := hegel.Draw(ht, hegel.Integers(0, 3))
	for range queryCount {
		name := []gwv1.HTTPHeaderName{"q-a", "q-b", "q-c"}[hegel.Draw(ht, hegel.Integers(0, 2))]
		value := []string{"a", "b"}[hegel.Draw(ht, hegel.Integers(0, 1))]
		match.QueryParams = append(match.QueryParams, gwv1.HTTPQueryParamMatch{
			Type:  new(gwv1.QueryParamMatchExact),
			Name:  name,
			Value: value,
		})
	}
	return &SortableRoute{
		Route: ir.HttpRouteRuleMatchIR{
			Match:            match,
			PrecedenceWeight: int32(hegel.Draw(ht, hegel.Integers(0, 2))),
		},
		RouteObject: &metav1.ObjectMeta{
			Name:              name,
			Namespace:         namespace,
			CreationTimestamp: metav1.NewTime(created),
		},
		Idx: hegel.Draw(ht, hegel.Integers(0, 2)),
	}
}

func TestSortableRouteCompareToPreorder(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		a, b, c := drawSortableRoute(ht), drawSortableRoute(ht), drawSortableRoute(ht)
		if got := a.CompareTo(a); got != 0 {
			ht.Fatalf("CompareTo(a, a) = %d, want 0", got)
		}
		ab, ba := sign(a.CompareTo(b)), sign(b.CompareTo(a))
		if ab != -ba {
			ht.Fatalf("antisymmetry failed: a=%d b=%d", ab, ba)
		}
		if a.CompareTo(b) <= 0 && b.CompareTo(c) <= 0 && a.CompareTo(c) > 0 {
			ht.Fatalf("transitivity failed: ab=%d bc=%d ac=%d", a.CompareTo(b), b.CompareTo(c), a.CompareTo(c))
		}
	}, hegel.WithTestCases(150))
}

func TestSortableRouteGatewayApiPathPrecedence(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		weight := int32(hegel.Draw(ht, hegel.Integers(0, 2)))
		value := []string{"/a", "/a/b", "/route"}[hegel.Draw(ht, hegel.Integers(0, 2))]
		object := &metav1.ObjectMeta{Name: "same", Namespace: "ns"}
		exactType := gwv1.PathMatchExact
		prefixType := gwv1.PathMatchPathPrefix
		exact := &SortableRoute{
			Route: ir.HttpRouteRuleMatchIR{
				Match:            gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: &exactType, Value: &value}},
				PrecedenceWeight: weight,
			},
			RouteObject: object,
		}
		prefixValue := value + "/longer"
		prefix := &SortableRoute{
			Route: ir.HttpRouteRuleMatchIR{
				Match:            gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: &prefixType, Value: &prefixValue}},
				PrecedenceWeight: weight,
			},
			RouteObject: object,
		}
		if exact.CompareTo(prefix) >= 0 {
			ht.Fatalf("exact path must sort before prefix: exact=%d", exact.CompareTo(prefix))
		}
		shortPrefix := &SortableRoute{
			Route: ir.HttpRouteRuleMatchIR{
				Match:            gwv1.HTTPRouteMatch{Path: &gwv1.HTTPPathMatch{Type: &prefixType, Value: &value}},
				PrecedenceWeight: weight,
			},
			RouteObject: object,
		}
		if prefix.CompareTo(shortPrefix) >= 0 {
			ht.Fatalf("longer prefix must sort before shorter prefix: compare=%d", prefix.CompareTo(shortPrefix))
		}
	}, hegel.WithTestCases(100))
}

func sign(n int) int {
	if n < 0 {
		return -1
	}
	if n > 0 {
		return 1
	}
	return 0
}
