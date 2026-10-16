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
	"fmt"
	"os"
	"strings"
	"testing"

	"hegel.dev/go/hegel"
)

func TestGetClusterDomainNameSearchWhitespace(t *testing.T) {
	if os.Getenv("KGATEWAY_HEGEL_KNOWN_BUGS") == "" {
		t.Skip("known bug: resolv.conf search fields separated by tabs are not split")
	}
	t.Setenv(clusterDomainEnvKey, "")
	hegel.Test(t, func(ht *hegel.T) {
		namespace := []string{"default", "gateway", "test"}[hegel.Draw(ht, hegel.Integers(0, 2))]
		domain := []string{"cluster.local", "example.com", "svc.test"}[hegel.Draw(ht, hegel.Integers(0, 2))]
		separatorCount := hegel.Draw(ht, hegel.Integers(1, 3))
		var separator strings.Builder
		for range separatorCount {
			if hegel.Draw(ht, hegel.Integers(0, 1)) == 0 {
				separator.WriteByte(' ')
			} else {
				separator.WriteByte('\t')
			}
		}
		resolvConf := fmt.Sprintf("nameserver 1.1.1.1\nsearch %s.svc.%s%ssvc.%s\n", namespace, domain, separator.String(), domain)
		if got := getClusterDomainName(strings.NewReader(resolvConf)); got != domain {
			ht.Fatalf("getClusterDomainName(%q) = %q, want %q", resolvConf, got, domain)
		}
	}, hegel.WithTestCases(100))
}
