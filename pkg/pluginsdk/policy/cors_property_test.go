package policy

import (
	"regexp"
	"strings"
	"testing"

	envoymatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	"hegel.dev/go/hegel"
)

var originTokens = []string{"https://", "http://", "*", ".", "example", "com", ":8080", "a"}

func drawOrigin(ht *hegel.T) string {
	count := hegel.Draw(ht, hegel.Integers(0, 8))
	var origin strings.Builder
	for range count {
		index := hegel.Draw(ht, hegel.Integers(0, len(originTokens)-1))
		origin.WriteString(originTokens[index])
	}
	return origin.String()
}

func originGlobMatch(pattern, candidate string) bool {
	if !strings.Contains(pattern, "*") {
		return pattern == candidate
	}
	parts := strings.Split(pattern, "*")
	position := 0
	partIndex := 0
	if !strings.HasPrefix(pattern, "*") {
		if !strings.HasPrefix(candidate, parts[0]) {
			return false
		}
		position = len(parts[0])
		partIndex = 1
	}
	trailingWildcard := strings.HasSuffix(pattern, "*")
	limit := len(parts)
	if !trailingWildcard {
		limit--
	}
	for i := partIndex; i < limit; i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		index := strings.Index(candidate[position:], part)
		if index < 0 {
			return false
		}
		position += index + len(part)
	}
	if trailingWildcard {
		return true
	}
	suffix := parts[len(parts)-1]
	if !strings.HasSuffix(candidate, suffix) {
		return false
	}
	return position <= len(candidate)-len(suffix)
}

func envoyStringMatcherMatches(matcher *envoymatcher.StringMatcher, candidate string) (bool, error) {
	switch pattern := matcher.GetMatchPattern().(type) {
	case *envoymatcher.StringMatcher_Exact:
		return candidate == pattern.Exact, nil
	case *envoymatcher.StringMatcher_Prefix:
		return strings.HasPrefix(candidate, pattern.Prefix), nil
	case *envoymatcher.StringMatcher_SafeRegex:
		re, err := regexp.Compile(pattern.SafeRegex.GetRegex())
		if err != nil {
			return false, err
		}
		return re.MatchString(candidate), nil
	default:
		return false, nil
	}
}

func TestConvertOriginToEnvoyStringMatcherMatchesGlobModel(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		origin, candidate := drawOrigin(ht), drawOrigin(ht)
		ht.Assume(origin != "*")
		matcher := ConvertOriginToEnvoyStringMatcher(origin)
		if matcher == nil {
			ht.Fatalf("matcher for %q is nil", origin)
		}
		got, err := envoyStringMatcherMatches(matcher, candidate)
		if err != nil {
			ht.Fatalf("evaluate matcher %v: %v", matcher, err)
		}
		want := originGlobMatch(origin, candidate)
		if got != want {
			ht.Fatalf("origin=%q candidate=%q matcher=%v got=%t model=%t", origin, candidate, matcher, got, want)
		}
	}, hegel.WithTestCases(150))
}
