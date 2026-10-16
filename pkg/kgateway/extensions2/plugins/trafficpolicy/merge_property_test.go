package trafficpolicy

import (
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	hegel "hegel.dev/go/hegel"
	"istio.io/istio/pkg/kube/krt"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"

	apiannotations "github.com/kgateway-dev/kgateway/v2/api/annotations"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/kgateway"
	"github.com/kgateway-dev/kgateway/v2/api/v1alpha1/shared"
	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/wellknown"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/collections"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/ir"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/krtutil"
	"github.com/kgateway-dev/kgateway/v2/pkg/pluginsdk/policy"
)

type trafficPolicyPropertyField struct {
	name    string
	origin  string
	inP4    bool
	get     func(*TrafficPolicy) any
	setSpec func(*kgateway.TrafficPolicySpec, int)
}

var trafficPolicyPropertyFields = []trafficPolicyPropertyField{
	{
		name:   "transformation",
		origin: "transformation",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.rustformation },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			spec.Transformation = &kgateway.TransformationPolicy{
				Request: &kgateway.Transform{
					Set: []kgateway.HeaderTransformation{{
						Name:  "x-property",
						Value: kgateway.InjaTemplate(fmt.Sprintf("value-%d", value)),
					}},
				},
			}
		},
	},
	{
		name:   "timeouts",
		origin: "timeouts",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.timeouts },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			spec.Timeouts = &shared.Timeouts{
				Request: &metav1.Duration{Duration: time.Duration(value+1) * time.Second},
			}
		},
	},
	{
		name:   "retry",
		origin: "retry",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.retry },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			spec.Retry = &kgateway.Retry{
				RetryOn:  []kgateway.RetryOnCondition{"5xx"},
				Attempts: int32(value + 1),
			}
		},
	},
	{
		name:   "headerModifiers",
		origin: "headerModifiers",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.headerModifiers },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			header := shared.HTTPHeader{
				Name:  ptr.To(gwv1.HTTPHeaderName("x-property")),
				Value: ptr.To(fmt.Sprintf("value-%d", value)),
			}
			spec.HeaderModifiers = &shared.HeaderModifiers{
				Request:  &shared.HTTPHeaderFilter{Set: []shared.HTTPHeader{header}},
				Response: &shared.HTTPHeaderFilter{Add: []shared.HTTPHeader{header}},
			}
		},
	},
	{
		name:   "cors",
		origin: "cors",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.cors },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			origin := "https://a.example"
			if value != 0 {
				origin = "https://b.example"
			}
			spec.Cors = &kgateway.CorsPolicy{
				HTTPCORSFilter: &gwv1.HTTPCORSFilter{
					AllowOrigins: []gwv1.CORSOrigin{gwv1.CORSOrigin(origin)},
				},
			}
		},
	},
	{
		name:   "csrf",
		origin: "csrf",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.csrf },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			spec.Csrf = &kgateway.CSRFPolicy{
				PercentageEnabled: ptr.To(int32(25 + value*25)),
			}
		},
	},
	{
		name:   "buffer",
		origin: "buffer",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.buffer },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			size := resource.MustParse(fmt.Sprintf("%dMi", value+1))
			spec.Buffer = &kgateway.Buffer{MaxRequestSize: &size}
		},
	},
	{
		name:   "httpUpgrade",
		origin: "httpUpgrade",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.httpUpgrade },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			upgradeType := "websocket"
			if value != 0 {
				upgradeType = "connect"
			}
			spec.HTTPUpgrade = []kgateway.ProtocolUpgradeConfig{{Type: upgradeType}}
		},
	},
	{
		name:   "localRateLimit",
		origin: "rateLimit.local",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.localRateLimit },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			spec.RateLimit = &kgateway.RateLimit{
				Local: &kgateway.LocalRateLimitPolicy{
					TokenBucket: &kgateway.TokenBucket{
						MaxTokens:    int32(value + 1),
						FillInterval: metav1.Duration{Duration: time.Second},
					},
				},
			}
		},
	},
	{
		name:   "autoHostRewrite",
		origin: "autoHostRewrite",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.autoHostRewrite },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			spec.AutoHostRewrite = ptr.To(value != 0)
		},
	},
	{
		name:   "urlRewrite",
		origin: "urlRewrite",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.urlRewrite },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			prefix := "a"
			if value != 0 {
				prefix = "b"
			}
			spec.UrlRewrite = &kgateway.URLRewrite{
				PathRegex: &kgateway.PathRegexRewrite{
					Pattern:      "/" + prefix + "/(.*)",
					Substitution: "/rewritten/\\1",
				},
			}
		},
	},
	{
		name:   "compression",
		origin: "compression",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.compression },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			library := kgateway.CompressionGzip
			if value != 0 {
				library = kgateway.CompressionBrotli
			}
			if spec.Compression == nil {
				spec.Compression = &kgateway.Compression{}
			}
			spec.Compression.ResponseCompression = &kgateway.ResponseCompression{
				Libraries: []kgateway.CompressionLibrary{library},
			}
		},
	},
	{
		name:   "decompression",
		origin: "decompression",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.decompression },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			library := kgateway.CompressionGzip
			if value != 0 {
				library = kgateway.CompressionBrotli
			}
			if spec.Compression == nil {
				spec.Compression = &kgateway.Compression{}
			}
			spec.Compression.RequestDecompression = &kgateway.RequestDecompression{
				Libraries: []kgateway.CompressionLibrary{library},
			}
		},
	},
	{
		name:   "faultInjection",
		origin: "faultInjection",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.faultInjection },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			spec.FaultInjection = &kgateway.FaultInjectionPolicy{
				Delay: &kgateway.FaultDelay{
					FixedDelay: metav1.Duration{Duration: time.Duration(value+1) * time.Millisecond},
				},
			}
		},
	},
	{
		name:   "statPrefix",
		origin: "statPrefix",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.statPrefix },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			spec.StatPrefix = ptr.To(fmt.Sprintf("prefix-%d", value))
		},
	},
	{
		name:   "tracing",
		origin: "tracing",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.tracing },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			spec.Tracing = &kgateway.RouteTracing{
				ClientSampling: ptr.To(int32(25 + value*25)),
			}
		},
	},
	{
		name:   "rbac",
		origin: "rbac",
		inP4:   true,
		get:    func(p *TrafficPolicy) any { return p.spec.rbac },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			method := "GET"
			if value != 0 {
				method = "POST"
			}
			spec.RBAC = &shared.Authorization{
				Action: shared.AuthorizationPolicyActionAllow,
				Policy: shared.AuthorizationPolicy{
					MatchExpressions: []shared.CELExpression{
						shared.CELExpression(fmt.Sprintf("request.auth.claims.groups == '%s'", method)),
					},
				},
			}
		},
	},
	{
		name:   "httpACL",
		origin: "httpACL",
		get:    func(p *TrafficPolicy) any { return p.spec.httpACL },
		setSpec: func(spec *kgateway.TrafficPolicySpec, value int) {
			cidr := shared.IPOrCIDR("10.0.0.0/8")
			if value != 0 {
				cidr = "192.168.0.0/16"
			}
			spec.ACL = &shared.ACLPolicy{
				DefaultAction: shared.ACLActionAllow,
				Rules: []shared.ACLRule{{
					CIDRs:  []shared.IPOrCIDR{cidr},
					Action: shared.ACLActionDeny,
				}},
			}
		},
	},
}

var inheritedPriorityValues = []apiannotations.InheritedPolicyPriorityValue{
	"",
	apiannotations.ShallowMergePreferParent,
	apiannotations.ShallowMergePreferChild,
	apiannotations.DeepMergePreferParent,
	apiannotations.DeepMergePreferChild,
}

func newTrafficPolicyPropertyConstructor(t *testing.T) *TrafficPolicyConstructor {
	t.Helper()
	ctx := t.Context()
	krtOptions := krtutil.NewKrtOptions(ctx.Done(), nil)
	commonCollections := &collections.CommonCollections{
		KrtOpts: krtOptions,
		GatewayExtensions: krt.NewStaticCollection[ir.GatewayExtension](
			nil,
			nil,
			krtOptions.ToOptions("GatewayExtensions")...,
		),
	}
	return NewTrafficPolicyConstructor(ctx, commonCollections)
}

func constructTrafficPolicyPropertyIR(
	constructor *TrafficPolicyConstructor,
	spec kgateway.TrafficPolicySpec,
	name string,
) (*TrafficPolicy, []error) {
	policyCR := &kgateway.TrafficPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:              name,
			Namespace:         "property-tests",
			CreationTimestamp: metav1.NewTime(time.Unix(1, 0).UTC()),
		},
		Spec: spec,
	}
	return constructor.ConstructIR(nil, policyCR)
}

func makeTrafficPolicyPropertyAtt(
	policyIR *TrafficPolicy,
	index int,
	hierarchicalPriority int,
	inheritedPriority apiannotations.InheritedPolicyPriorityValue,
) ir.PolicyAtt {
	return ir.PolicyAtt{
		GroupKind: wellknown.TrafficPolicyGVK.GroupKind(),
		PolicyIr:  policyIR,
		PolicyRef: &ir.AttachedPolicyRef{
			Group:     wellknown.TrafficPolicyGVK.Group,
			Kind:      wellknown.TrafficPolicyGVK.Kind,
			Namespace: "property-tests",
			Name:      fmt.Sprintf("p%d", index),
		},
		InheritedPolicyPriority: inheritedPriority,
		HierarchicalPriority:    hierarchicalPriority,
		Generation:              1,
	}
}

func buildTrafficPolicyPropertyAtts(
	ht *hegel.T,
	constructor *TrafficPolicyConstructor,
	specs []kgateway.TrafficPolicySpec,
	sameHierarchy bool,
) ([]ir.PolicyAtt, []*TrafficPolicy) {
	atts := make([]ir.PolicyAtt, 0, len(specs))
	twins := make([]*TrafficPolicy, 0, len(specs))
	for i, spec := range specs {
		policyIR, errs := constructTrafficPolicyPropertyIR(constructor, spec, fmt.Sprintf("p%d", i))
		if len(errs) != 0 {
			ht.Fatalf("construct p%d: %v; spec=%+v", i, errs, spec)
		}
		twin, twinErrs := constructTrafficPolicyPropertyIR(constructor, spec, fmt.Sprintf("p%d", i))
		if len(twinErrs) != 0 {
			ht.Fatalf("construct twin p%d: %v; spec=%+v", i, twinErrs, spec)
		}

		hierarchicalPriority := hegel.Draw(ht, hegel.Integers(0, 2))
		if sameHierarchy {
			hierarchicalPriority = 1
		}
		inheritedPriority := inheritedPriorityValues[hegel.Draw(ht, hegel.Integers(0, len(inheritedPriorityValues)-1))]
		atts = append(atts, makeTrafficPolicyPropertyAtt(policyIR, i, hierarchicalPriority, inheritedPriority))
		twins = append(twins, twin)
	}
	return atts, twins
}

func drawTrafficPolicyPropertySpec(
	ht *hegel.T,
	allowBuffer, allowHTTPUpgrade, allowACL bool,
) kgateway.TrafficPolicySpec {
	spec := kgateway.TrafficPolicySpec{}
	bufferSelected := allowBuffer && hegel.Draw(ht, hegel.Integers(0, 1)) == 1
	upgradeSelected := allowHTTPUpgrade && hegel.Draw(ht, hegel.Integers(0, 1)) == 1
	if bufferSelected && upgradeSelected {
		if hegel.Draw(ht, hegel.Integers(0, 1)) == 0 {
			bufferSelected = false
		} else {
			upgradeSelected = false
		}
	}

	for _, field := range trafficPolicyPropertyFields {
		selected := hegel.Draw(ht, hegel.Integers(0, 1)) == 1
		switch field.name {
		case "buffer":
			selected = bufferSelected
		case "httpUpgrade":
			selected = upgradeSelected
		case "httpACL":
			selected = allowACL && selected
		}
		if selected {
			field.setSpec(&spec, hegel.Draw(ht, hegel.Integers(0, 1)))
		}
	}
	return spec
}

func trafficPolicyPropertyFieldByName(name string) trafficPolicyPropertyField {
	for _, field := range trafficPolicyPropertyFields {
		if field.name == name {
			return field
		}
	}
	panic("unknown TrafficPolicy property field: " + name)
}

func trafficPolicyPropertyFieldIsSet(field trafficPolicyPropertyField, policyIR *TrafficPolicy) bool {
	switch field.name {
	case "transformation":
		return policyIR.spec.rustformation != nil
	case "timeouts":
		return policyIR.spec.timeouts != nil
	case "retry":
		return policyIR.spec.retry != nil
	case "headerModifiers":
		return policyIR.spec.headerModifiers != nil
	case "cors":
		return policyIR.spec.cors != nil
	case "csrf":
		return policyIR.spec.csrf != nil
	case "buffer":
		return policyIR.spec.buffer != nil
	case "httpUpgrade":
		return policyIR.spec.httpUpgrade != nil
	case "localRateLimit":
		return policyIR.spec.localRateLimit != nil
	case "autoHostRewrite":
		return policyIR.spec.autoHostRewrite != nil
	case "urlRewrite":
		return policyIR.spec.urlRewrite != nil
	case "compression":
		return policyIR.spec.compression != nil
	case "decompression":
		return policyIR.spec.decompression != nil
	case "faultInjection":
		return policyIR.spec.faultInjection != nil
	case "statPrefix":
		return policyIR.spec.statPrefix != nil
	case "tracing":
		return policyIR.spec.tracing != nil
	case "rbac":
		return policyIR.spec.rbac != nil
	case "httpACL":
		return policyIR.spec.httpACL != nil
	default:
		panic("unknown TrafficPolicy property field: " + field.name)
	}
}

func trafficPolicyPropertyFieldEquals(field trafficPolicyPropertyField, a, b any) bool {
	if field.name == "rbac" {
		return a.(*rbacIR).Equals(b.(*rbacIR))
	}
	return a.(PolicySubIR).Equals(b.(PolicySubIR))
}

func drawTrafficPolicyMergeSettings(ht *hegel.T) string {
	strategies := []string{"", "DeepMerge", "ShallowMerge"}
	mergeOpts := mergeOpts{
		TrafficPolicy: TrafficPolicyMergeOpts{
			ExtAuth:        strategies[hegel.Draw(ht, hegel.Integers(0, len(strategies)-1))],
			ExtProc:        strategies[hegel.Draw(ht, hegel.Integers(0, len(strategies)-1))],
			Transformation: strategies[hegel.Draw(ht, hegel.Integers(0, len(strategies)-1))],
			ACL:            strategies[hegel.Draw(ht, hegel.Integers(0, len(strategies)-1))],
		},
	}
	if mergeOpts.TrafficPolicy.ExtAuth == "" &&
		mergeOpts.TrafficPolicy.ExtProc == "" &&
		mergeOpts.TrafficPolicy.Transformation == "" &&
		mergeOpts.TrafficPolicy.ACL == "" {
		return ""
	}
	encoded, err := json.Marshal(mergeOpts)
	if err != nil {
		ht.Fatalf("marshal merge options: %v", err)
	}
	return string(encoded)
}

func mergeTrafficPolicyPropertyAtts(atts []ir.PolicyAtt, settings string) ir.PolicyAtt {
	return policy.MergePolicies[TrafficPolicy](atts, mergeTrafficPolicies, settings)
}

func mergedTrafficPolicy(ht *hegel.T, att ir.PolicyAtt) *TrafficPolicy {
	result, ok := att.PolicyIr.(*TrafficPolicy)
	if !ok {
		ht.Fatalf("merged policy has type %T", att.PolicyIr)
	}
	return result
}

func TestTrafficPolicyMergeInputsImmutableAndRepeatableProperty(t *testing.T) {
	constructor := newTrafficPolicyPropertyConstructor(t)
	hegel.Test(t, func(ht *hegel.T) {
		count := hegel.Draw(ht, hegel.Integers(1, 4))
		specs := make([]kgateway.TrafficPolicySpec, count)
		for i := range specs {
			specs[i] = drawTrafficPolicyPropertySpec(ht, true, true, true)
		}
		atts, twins := buildTrafficPolicyPropertyAtts(ht, constructor, specs, false)
		settings := drawTrafficPolicyMergeSettings(ht)

		first := mergeTrafficPolicyPropertyAtts(atts, settings)
		second := mergeTrafficPolicyPropertyAtts(atts, settings)
		for i, att := range atts {
			input := att.PolicyIr.(*TrafficPolicy)
			if !input.Equals(twins[i]) {
				ht.Fatalf("merge mutated input p%d: input=%+v twin=%+v settings=%q", i, input, twins[i], settings)
			}
		}
		firstIR := mergedTrafficPolicy(ht, first)
		secondIR := mergedTrafficPolicy(ht, second)
		if !firstIR.Equals(secondIR) {
			ht.Fatalf("repeated merge differed: first=%+v second=%+v settings=%q", firstIR, secondIR, settings)
		}
	}, hegel.WithTestCases(80), hegel.WithDatabase(""))
}

func TestTrafficPolicyMergeBufferAndHTTPUpgradeExclusionProperty(t *testing.T) {
	constructor := newTrafficPolicyPropertyConstructor(t)
	hegel.Test(t, func(ht *hegel.T) {
		count := hegel.Draw(ht, hegel.Integers(2, 4))
		specs := make([]kgateway.TrafficPolicySpec, count)
		var bufferSpec, upgradeSpec kgateway.TrafficPolicySpec
		trafficPolicyPropertyFieldByName("buffer").setSpec(&bufferSpec, hegel.Draw(ht, hegel.Integers(0, 1)))
		trafficPolicyPropertyFieldByName("httpUpgrade").setSpec(&upgradeSpec, hegel.Draw(ht, hegel.Integers(0, 1)))
		if hegel.Draw(ht, hegel.Integers(0, 1)) == 0 {
			specs[0], specs[1] = bufferSpec, upgradeSpec
		} else {
			specs[0], specs[1] = upgradeSpec, bufferSpec
		}
		for i := 2; i < count; i++ {
			specs[i] = drawTrafficPolicyPropertySpec(ht, true, true, true)
		}

		atts, _ := buildTrafficPolicyPropertyAtts(ht, constructor, specs, false)
		merged := mergedTrafficPolicy(ht, mergeTrafficPolicyPropertyAtts(atts, drawTrafficPolicyMergeSettings(ht)))
		if bufferEnabled(merged.spec.buffer) && merged.spec.httpUpgrade != nil {
			ht.Fatalf("merged enabled buffer and HTTP upgrade together: buffer=%+v httpUpgrade=%+v", merged.spec.buffer, merged.spec.httpUpgrade)
		}
	}, hegel.WithTestCases(80), hegel.WithDatabase(""))
}

func skipKnownTrafficPolicyMergeBug(t *testing.T, reason string) {
	t.Helper()
	if os.Getenv("KGATEWAY_HEGEL_KNOWN_BUGS") == "" {
		t.Skip("known bug: " + reason)
	}
}

func TestTrafficPolicyMergeSinglePolicyEqualsProperty(t *testing.T) {
	skipKnownTrafficPolicyMergeBug(t, "MergePolicies does not preserve the TrafficPolicy creation time")
	constructor := newTrafficPolicyPropertyConstructor(t)
	hegel.Test(t, func(ht *hegel.T) {
		spec := drawTrafficPolicyPropertySpec(ht, true, true, true)
		atts, _ := buildTrafficPolicyPropertyAtts(ht, constructor, []kgateway.TrafficPolicySpec{spec}, false)
		input := atts[0].PolicyIr.(*TrafficPolicy)
		merged := mergedTrafficPolicy(ht, mergeTrafficPolicyPropertyAtts(atts, ""))
		if !merged.Equals(input) {
			ht.Fatalf("single-policy merge differed: spec=%+v inputCreationTime=%s mergedCreationTime=%s input=%+v merged=%+v",
				spec, input.CreationTime(), merged.CreationTime(), input, merged)
		}
	}, hegel.WithTestCases(40), hegel.WithDatabase(""))
}

func TestTrafficPolicyMergeSinglePolicyEqualsPropertyCounterexample(t *testing.T) {
	skipKnownTrafficPolicyMergeBug(t, "MergePolicies does not preserve the TrafficPolicy creation time")
	constructor := newTrafficPolicyPropertyConstructor(t)
	input, errs := constructTrafficPolicyPropertyIR(constructor, kgateway.TrafficPolicySpec{}, "p0")
	if len(errs) != 0 {
		t.Fatalf("construct minimal policy: %v", errs)
	}
	att := makeTrafficPolicyPropertyAtt(input, 0, 0, "")
	merged := policy.MergePolicies[TrafficPolicy]([]ir.PolicyAtt{att}, mergeTrafficPolicies, "")
	mergedIR, ok := merged.PolicyIr.(*TrafficPolicy)
	if !ok {
		t.Fatalf("merged policy has type %T", merged.PolicyIr)
	}
	if !mergedIR.Equals(input) {
		t.Fatalf("single empty policy should compare equal: inputCreationTime=%s mergedCreationTime=%s input=%+v merged=%+v",
			input.CreationTime(), mergedIR.CreationTime(), input, mergedIR)
	}
}

func TestTrafficPolicyMergeSameHierarchyPrecedenceProperty(t *testing.T) {
	constructor := newTrafficPolicyPropertyConstructor(t)
	hegel.Test(t, func(ht *hegel.T) {
		count := hegel.Draw(ht, hegel.Integers(1, 4))
		bufferAllowed := hegel.Draw(ht, hegel.Integers(0, 1)) == 0
		specs := make([]kgateway.TrafficPolicySpec, count)
		for i := range specs {
			specs[i] = drawTrafficPolicyPropertySpec(ht, bufferAllowed, !bufferAllowed, false)
		}
		atts, _ := buildTrafficPolicyPropertyAtts(ht, constructor, specs, true)
		merged := mergedTrafficPolicy(ht, mergeTrafficPolicyPropertyAtts(atts, ""))

		for _, field := range trafficPolicyPropertyFields {
			if !field.inP4 {
				continue
			}
			var firstSet any
			firstSetInInput := false
			for _, att := range atts {
				source := att.PolicyIr.(*TrafficPolicy)
				if trafficPolicyPropertyFieldIsSet(field, source) {
					firstSet = field.get(source)
					firstSetInInput = true
					break
				}
			}
			got := field.get(merged)
			gotIsSet := trafficPolicyPropertyFieldIsSet(field, merged)
			if firstSetInInput != gotIsSet || firstSetInInput && !trafficPolicyPropertyFieldEquals(field, got, firstSet) {
				ht.Fatalf("same-hierarchy %s did not keep the first set field: got=%+v want=%+v specs=%+v",
					field.name, got, firstSet, specs)
			}
		}
	}, hegel.WithTestCases(80), hegel.WithDatabase(""))
}

func TestTrafficPolicyEqualsProperties(t *testing.T) {
	constructor := newTrafficPolicyPropertyConstructor(t)
	hegel.Test(t, func(ht *hegel.T) {
		spec := drawTrafficPolicyPropertySpec(ht, true, true, true)
		first, firstErrs := constructTrafficPolicyPropertyIR(constructor, spec, "p0")
		if len(firstErrs) != 0 {
			ht.Fatalf("construct first policy: %v; spec=%+v", firstErrs, spec)
		}
		second, secondErrs := constructTrafficPolicyPropertyIR(constructor, spec, "p0")
		if len(secondErrs) != 0 {
			ht.Fatalf("construct second policy: %v; spec=%+v", secondErrs, spec)
		}
		if !first.Equals(second) || !second.Equals(first) {
			ht.Fatalf("identical specs produced unequal IRs: spec=%+v first=%+v second=%+v", spec, first, second)
		}

		field := trafficPolicyPropertyFields[hegel.Draw(ht, hegel.Integers(0, len(trafficPolicyPropertyFields)-1))]
		value := hegel.Draw(ht, hegel.Integers(0, 1))
		firstSpec, secondSpec := kgateway.TrafficPolicySpec{}, kgateway.TrafficPolicySpec{}
		field.setSpec(&firstSpec, value)
		field.setSpec(&secondSpec, 1-value)
		first, firstErrs = constructTrafficPolicyPropertyIR(constructor, firstSpec, "p0")
		second, secondErrs = constructTrafficPolicyPropertyIR(constructor, secondSpec, "p0")
		if len(firstErrs) != 0 || len(secondErrs) != 0 {
			ht.Fatalf("construct changed %s field: first errors=%v second errors=%v", field.name, firstErrs, secondErrs)
		}
		if first.Equals(second) || second.Equals(first) {
			ht.Fatalf("Equals did not distinguish %s: firstSpec=%+v secondSpec=%+v first=%+v second=%+v",
				field.name, firstSpec, secondSpec, first, second)
		}
	}, hegel.WithTestCases(60), hegel.WithDatabase(""))
}

func TestTrafficPolicyMergeOriginsProperty(t *testing.T) {
	constructor := newTrafficPolicyPropertyConstructor(t)
	hegel.Test(t, func(ht *hegel.T) {
		count := hegel.Draw(ht, hegel.Integers(1, 4))
		bufferAllowed := hegel.Draw(ht, hegel.Integers(0, 1)) == 0
		specs := make([]kgateway.TrafficPolicySpec, count)
		for i := range specs {
			specs[i] = drawTrafficPolicyPropertySpec(ht, bufferAllowed, !bufferAllowed, false)
		}
		atts, _ := buildTrafficPolicyPropertyAtts(ht, constructor, specs, true)
		result := mergeTrafficPolicyPropertyAtts(atts, "")
		merged := mergedTrafficPolicy(ht, result)

		for _, field := range trafficPolicyPropertyFields {
			if !field.inP4 || !trafficPolicyPropertyFieldIsSet(field, merged) {
				continue
			}
			var expectedOrigin string
			for _, att := range atts {
				if trafficPolicyPropertyFieldIsSet(field, att.PolicyIr.(*TrafficPolicy)) {
					expectedOrigin = att.PolicyRef.ID()
					break
				}
			}
			if expectedOrigin == "" || !slices.Contains(result.MergeOrigins.Get(field.origin), expectedOrigin) {
				ht.Fatalf("merge origin for %s missing supplier %q: origins=%v specs=%+v",
					field.name, expectedOrigin, result.MergeOrigins.Get(field.origin), specs)
			}
		}
	}, hegel.WithTestCases(80), hegel.WithDatabase(""))
}
