package delegation

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"hegel.dev/go/hegel"
	gwv1 "sigs.k8s.io/gateway-api/apis/v1"
)

var pathValueTokens = []string{"", "/", "x", "abc", "0", "/v1", "|", ".*", "|/admin.*", "(", ")", "?"}

func drawPathValue(ht *hegel.T) string {
	count := hegel.Draw(ht, hegel.Integers(1, 4))
	var value strings.Builder
	for range count {
		token := hegel.Draw(ht, hegel.Integers(0, len(pathValueTokens)-1))
		value.WriteString(pathValueTokens[token])
	}
	return value.String()
}

func drawParentPath(ht *hegel.T) string {
	if hegel.Draw(ht, hegel.Integers(0, 3)) == 0 {
		return "/"
	}
	count := hegel.Draw(ht, hegel.Integers(1, 3))
	segments := make([]string, count)
	for i := range segments {
		length := hegel.Draw(ht, hegel.Integers(1, 6))
		for range length {
			index := hegel.Draw(ht, hegel.Integers(0, 35))
			if index < 26 {
				segments[i] += string(rune('a' + index))
			} else {
				segments[i] += string(rune('0' + index - 26))
			}
		}
	}
	return "/" + strings.Join(segments, "/")
}

func drawChildMatch(ht *hegel.T, parentPath string) gwv1.HTTPRouteMatch {
	types := []gwv1.PathMatchType{
		gwv1.PathMatchPathPrefix,
		gwv1.PathMatchExact,
		gwv1.PathMatchRegularExpression,
	}
	pathType := types[hegel.Draw(ht, hegel.Integers(0, len(types)-1))]
	pathValue := parentPath + drawPathValue(ht)
	return gwv1.HTTPRouteMatch{
		Path: &gwv1.HTTPPathMatch{
			Type:  &pathType,
			Value: &pathValue,
		},
	}
}

func envoyMatches(match gwv1.HTTPRouteMatch, reqPath string) (bool, error) {
	if match.Path == nil || match.Path.Value == nil {
		return false, nil
	}
	pathValue := *match.Path.Value
	pathType := gwv1.PathMatchPathPrefix
	if match.Path.Type != nil {
		pathType = *match.Path.Type
	}
	switch pathType {
	case gwv1.PathMatchPathPrefix:
		if len(pathValue) > 1 {
			pathValue = strings.TrimSuffix(pathValue, "/")
		}
		separatedPathRegex := regexp.MustCompile("^[^?#]+[^?#/]$")
		if separatedPathRegex.MatchString(pathValue) {
			return reqPath == pathValue || strings.HasPrefix(reqPath, pathValue+"/"), nil
		}
		return strings.HasPrefix(reqPath, pathValue), nil
	case gwv1.PathMatchExact:
		return reqPath == pathValue, nil
	case gwv1.PathMatchRegularExpression:
		re, err := regexp.Compile("^(?:" + pathValue + ")$")
		if err != nil {
			return false, err
		}
		return re.MatchString(reqPath), nil
	default:
		return false, fmt.Errorf("unknown path type %q", pathType)
	}
}

func drawRequestPath(ht *hegel.T, parentPath string) string {
	switch hegel.Draw(ht, hegel.Integers(0, 2)) {
	case 0:
		length := hegel.Draw(ht, hegel.Integers(0, 12))
		const alphabet = "/abcdefghijklmnopqrstuvwxyz0123456789-._~"
		var suffix strings.Builder
		for range length {
			index := hegel.Draw(ht, hegel.Integers(0, len(alphabet)-1))
			suffix.WriteByte(alphabet[index])
		}
		return parentPath + suffix.String()
	case 1:
		return "/admin/x"
	default:
		length := hegel.Draw(ht, hegel.Integers(1, 6))
		var segment strings.Builder
		for range length {
			index := hegel.Draw(ht, hegel.Integers(0, 35))
			if index < 26 {
				segment.WriteByte(byte('a' + index))
			} else {
				segment.WriteByte(byte('0' + index - 26))
			}
		}
		return "/" + segment.String()
	}
}

func TestDelegatedRouteMatchContainsParent(t *testing.T) {
	if os.Getenv("KGATEWAY_HEGEL_KNOWN_BUGS") == "" {
		t.Skip("known bug: regex child matches paths outside the parent's slash-delimited prefix")
	}
	hegel.Test(t, func(ht *hegel.T) {
		parentPath := drawParentPath(ht)
		pathType := gwv1.PathMatchPathPrefix
		parent := gwv1.HTTPRouteMatch{
			Path: &gwv1.HTTPPathMatch{Type: &pathType, Value: &parentPath},
		}
		child := drawChildMatch(ht, parentPath)
		reqPath := drawRequestPath(ht, parentPath)
		childMatches, err := envoyMatches(child, reqPath)
		if err != nil {
			ht.Assume(false)
		}
		if !IsDelegatedRouteMatch(parent, child) || !childMatches {
			return
		}
		parentMatches, err := envoyMatches(parent, reqPath)
		if err != nil {
			ht.Assume(false)
		}
		if !parentMatches {
			ht.Fatalf("parent=%q child=%q type=%s request=%q", parentPath, *child.Path.Value, *child.Path.Type, reqPath)
		}
	}, hegel.WithTestCases(100))
}

func TestInheritedDelegatedMatchesStayWithinParent(t *testing.T) {
	if os.Getenv("KGATEWAY_HEGEL_KNOWN_BUGS") == "" {
		t.Skip("known bug: inherited regex alternation can match outside the parent path")
	}
	hegel.Test(t, func(ht *hegel.T) {
		parentPath := drawParentPath(ht)
		pathType := gwv1.PathMatchPathPrefix
		parent := gwv1.HTTPRouteMatch{
			Path: &gwv1.HTTPPathMatch{Type: &pathType, Value: &parentPath},
		}
		count := hegel.Draw(ht, hegel.Integers(0, 2))
		children := make([]gwv1.HTTPRouteMatch, count)
		for i := range children {
			children[i] = drawChildMatch(ht, parentPath)
		}
		matches := FilterDelegatedRuleMatches(parent, children, true)
		reqPath := drawRequestPath(ht, parentPath)
		for _, match := range matches {
			if match.Path == nil || match.Path.Value == nil || !strings.HasPrefix(*match.Path.Value, "/") {
				ht.Fatalf("inherited match path must start with '/': %#v", match.Path)
			}
			matchesRequest, err := envoyMatches(match, reqPath)
			if err != nil {
				ht.Assume(false)
			}
			if !matchesRequest {
				continue
			}
			parentMatches, err := envoyMatches(parent, reqPath)
			if err != nil {
				ht.Assume(false)
			}
			if !parentMatches {
				ht.Fatalf("parent=%q inherited=%q type=%s request=%q", parentPath, *match.Path.Value, *match.Path.Type, reqPath)
			}
		}
	}, hegel.WithTestCases(100))
}
