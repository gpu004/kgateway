package utils

import (
	"strings"
	"testing"
	"unicode/utf8"

	"hegel.dev/go/hegel"
)

func TestSanitizeHeaderNameProperties(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		input := hegel.Draw(ht, hegel.Text().MaxSize(48))
		got := SanitizeHeaderName(input)
		if got == "" {
			ht.Fatalf("SanitizeHeaderName(%q) returned empty", input)
		}
		for i := 0; i < len(got); i++ {
			if !isHeaderTchar(got[i]) {
				ht.Fatalf("SanitizeHeaderName(%q) returned non-tchar byte %q", input, got[i])
			}
		}
		if again := SanitizeHeaderName(got); again != got {
			ht.Fatalf("SanitizeHeaderName is not idempotent: %q != %q", got, again)
		}
	}, hegel.WithTestCases(100))
}

func isHeaderTchar(b byte) bool {
	return b >= 'A' && b <= 'Z' ||
		b >= 'a' && b <= 'z' ||
		b >= '0' && b <= '9' ||
		strings.ContainsRune("!#$%&'*+-.^_`|~", rune(b))
}

func TestSanitizeCookieNameProperties(t *testing.T) {
	hegel.Test(t, func(ht *hegel.T) {
		input := hegel.Draw(ht, hegel.Text().MaxSize(48))
		got := SanitizeCookieName(input)
		if got == "" {
			ht.Fatalf("SanitizeCookieName(%q) returned empty", input)
		}
		for _, r := range got {
			if r > 127 || r < 32 || r == 127 || strings.ContainsRune("()<>@,;:\\\"/[]?={} \t", r) {
				ht.Fatalf("SanitizeCookieName(%q) returned invalid rune %q", input, r)
			}
		}
		if !utf8.ValidString(got) {
			ht.Fatalf("SanitizeCookieName(%q) returned invalid UTF-8", input)
		}
		if again := SanitizeCookieName(got); again != got {
			ht.Fatalf("SanitizeCookieName is not idempotent: %q != %q", got, again)
		}
	}, hegel.WithTestCases(100))
}
