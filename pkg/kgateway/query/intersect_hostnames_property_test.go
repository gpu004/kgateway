package query_test

import (
	"strings"
	"testing"

	"hegel.dev/go/hegel"

	"github.com/kgateway-dev/kgateway/v2/pkg/kgateway/query"
)

func drawHostname(ht *hegel.T) string {
	if hegel.Draw(ht, hegel.Integers(0, 4)) == 0 {
		return ""
	}
	count := hegel.Draw(ht, hegel.Integers(1, 4))
	labels := []string{"a", "b", "ex", "com"}
	var host strings.Builder
	if hegel.Draw(ht, hegel.Integers(0, 1)) == 1 {
		host.WriteString("*.")
	}
	for i := range count {
		if i > 0 {
			host.WriteByte('.')
		}
		host.WriteString(labels[hegel.Draw(ht, hegel.Integers(0, len(labels)-1))])
	}
	return host.String()
}

func hostnameMatches(pattern, hostname string) bool {
	if pattern == "" {
		return true
	}
	if !strings.HasPrefix(pattern, "*.") {
		return hostname == pattern
	}
	suffix := pattern[1:]
	if !strings.HasSuffix(hostname, suffix) {
		return false
	}
	return len(hostname) > len(suffix)
}

func hostnameIntersectionExists(a, b string) bool {
	candidates := []string{"a"}
	for _, pattern := range []string{a, b} {
		if pattern == "" {
			continue
		}
		if strings.HasPrefix(pattern, "*.") {
			suffix := pattern[2:]
			candidates = append(candidates, "a."+suffix, "a.b."+suffix)
		} else {
			candidates = append(candidates, pattern)
		}
	}
	for _, candidate := range candidates {
		if hostnameMatches(a, candidate) && hostnameMatches(b, candidate) {
			return true
		}
	}
	return false
}

func hostnameIntersection(a, b string) string {
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	aWildcard := strings.HasPrefix(a, "*.")
	bWildcard := strings.HasPrefix(b, "*.")
	switch {
	case !aWildcard:
		return a
	case !bWildcard:
		return b
	case len(a) >= len(b):
		return a
	default:
		return b
	}
}

func TestIntersectHostnamesProperties(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		a, b := drawHostname(ht), drawHostname(ht)
		got, ok := query.IntersectHostnames(a, b)
		_, reverseOK := query.IntersectHostnames(b, a)
		if ok != reverseOK {
			ht.Fatalf("intersection symmetry failed: %q, %q => %t and %t", a, b, ok, reverseOK)
		}
		wantOK := hostnameIntersectionExists(a, b)
		if ok != wantOK {
			ht.Fatalf("IntersectHostnames(%q, %q) ok=%t, model=%t", a, b, ok, wantOK)
		}
		if ok && got != hostnameIntersection(a, b) {
			ht.Fatalf("IntersectHostnames(%q, %q) = %q, want more-specific %q", a, b, got, hostnameIntersection(a, b))
		}
	}, hegel.WithTestCases(150))
}
