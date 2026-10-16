/*
Copyright 2019 The Knative Authors

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package kubeutils

import (
	"os"
	"strings"
	"testing"

	"hegel.dev/go/hegel"
	helmchart "helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chartutil"
	helmengine "helm.sh/helm/v3/pkg/engine"
	"k8s.io/apimachinery/pkg/util/validation"
)

func drawDNS1123Subdomain(ht *hegel.T) string {
	length := hegel.Draw(ht, hegel.Integers(1, 253))
	minLabels := (length + 1 + 63) / 64
	maxLabels := (length + 1) / 2
	if maxLabels > 5 {
		maxLabels = 5
	}
	labelCount := hegel.Draw(ht, hegel.Integers(minLabels, maxLabels))
	characters := length - labelCount + 1
	labels := make([]string, labelCount)
	for i := range labels {
		remainingLabels := labelCount - i - 1
		minLength := characters - 63*remainingLabels
		if minLength < 1 {
			minLength = 1
		}
		maxLength := characters - remainingLabels
		if maxLength > 63 {
			maxLength = 63
		}
		labelLength := hegel.Draw(ht, hegel.Integers(minLength, maxLength))
		characters -= labelLength
		labels[i] = drawDNSLabel(ht, labelLength)
	}
	return strings.Join(labels, ".")
}

func drawDNSLabel(ht *hegel.T, length int) string {
	const alphanumeric = "abcdefghijklmnopqrstuvwxyz0123456789"
	const middle = "abcdefghijklmnopqrstuvwxyz0123456789-"
	if length == 1 {
		return string(alphanumeric[hegel.Draw(ht, hegel.Integers(0, len(alphanumeric)-1))])
	}
	var label strings.Builder
	label.WriteByte(alphanumeric[hegel.Draw(ht, hegel.Integers(0, len(alphanumeric)-1))])
	for range length - 2 {
		label.WriteByte(middle[hegel.Draw(ht, hegel.Integers(0, len(middle)-1))])
	}
	label.WriteByte(alphanumeric[hegel.Draw(ht, hegel.Integers(0, len(alphanumeric)-1))])
	return label.String()
}

func renderHelmSafeLabelValue(name string) (string, error) {
	const templatePath = "../../kgateway/helm/envoy/templates/_helpers.tpl"
	source, err := os.ReadFile(templatePath)
	if err != nil {
		return "", err
	}
	templateText := `{{ include "kgateway.gateway.safeLabelValue" .Values.value }}`
	chart := &helmchart.Chart{
		Metadata: &helmchart.Metadata{Name: "safe-label-property", Version: "1.0.0"},
		Templates: []*helmchart.File{{
			Name: "templates/_helpers.tpl",
			Data: source,
		}, {
			Name: "templates/property.txt",
			Data: []byte(templateText),
		}},
	}
	rendered, err := helmengine.Render(chart, chartutil.Values{
		"Values": chartutil.Values{"value": name},
	})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(rendered["safe-label-property/templates/property.txt"]), nil
}

func TestSafeGatewayLabelValueProperties(t *testing.T) {
	helmSource, err := os.ReadFile("../../kgateway/helm/envoy/templates/_helpers.tpl")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(helmSource), `define "kgateway.gateway.safeLabelValue"`) {
		t.Fatal("Helm safe-label helper not found")
	}
	hegel.Test(t, func(ht *hegel.T) {
		name := drawDNS1123Subdomain(ht)
		if validation.IsDNS1123Subdomain(name) != nil {
			ht.Fatalf("generator produced invalid DNS1123 subdomain %q", name)
		}
		got := SafeGatewayLabelValue(name)
		if validation.IsValidLabelValue(got) != nil {
			ht.Fatalf("output %q is not a valid label value", got)
		}
		if len(got) > 63 {
			ht.Fatalf("output length %d exceeds 63: %q", len(got), got)
		}
		if len(name) <= 63 && got != name {
			ht.Fatalf("short name changed: input=%q output=%q", name, got)
		}
		if again := SafeGatewayLabelValue(name); again != got {
			ht.Fatalf("function is not deterministic: %q != %q", got, again)
		}
		helmValue, err := renderHelmSafeLabelValue(name)
		if err != nil {
			ht.Fatalf("render Helm helper: %v", err)
		}
		if got != helmValue {
			ht.Fatalf("Go output %q differs from Helm output %q for %q", got, helmValue, name)
		}
	}, hegel.WithTestCases(100))
}
