package arnutils

import (
	"strings"
	"testing"

	"hegel.dev/go/hegel"
)

func drawTokenString(ht *hegel.T, tokens []string, countMin, countMax int) string {
	count := hegel.Draw(ht, hegel.Integers(countMin, countMax))
	var value strings.Builder
	for range count {
		index := hegel.Draw(ht, hegel.Integers(0, len(tokens)-1))
		value.WriteString(tokens[index])
	}
	return value.String()
}

func TestIsARNMatchesParse(t *testing.T) {
	tokens := []string{"arn", ":", "aws", "s3", "", "x"}
	hegel.Test(t, func(ht *hegel.T) {
		value := drawTokenString(ht, tokens, 0, 12)
		_, err := Parse(value)
		if got, want := IsARN(value), err == nil; got != want {
			ht.Fatalf("IsARN(%q) = %t, Parse error=%v", value, got, err)
		}
	}, hegel.WithTestCases(150))
}

func TestARNStringRoundTrip(t *testing.T) {
	fields := []string{"", "aws", "s3", "x"}
	hegel.Test(t, func(ht *hegel.T) {
		field := func() string {
			return drawTokenString(ht, fields, 0, 3)
		}
		resource := drawTokenString(ht, []string{"", "aws", "s3", "x", ":"}, 0, 6)
		original := ARN{
			Partition: field(),
			Service:   field(),
			Region:    field(),
			AccountID: field(),
			Resource:  resource,
		}
		parsed, err := Parse(original.String())
		if err != nil {
			ht.Fatalf("Parse(%q): %v", original.String(), err)
		}
		if parsed != original {
			ht.Fatalf("Parse(%q) = %#v, want %#v", original.String(), parsed, original)
		}
	}, hegel.WithTestCases(100))
}
