//
// Copyright (c) 2023 Red Hat, Inc.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package helper

import (
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/onsi/gomega/types"
)

type ApiEndpointTest struct {
	Endpoint               string
	ExpectedHttpStatusCode int
	BodyMatcher            types.GomegaMatcher
}

func VerifyBackstagePodStatus(g Gomega, ns string, crName string, expectedStatus string) {
	cmd := exec.Command("kubectl", "get", "pods",
		"-l", "rhdh.redhat.com/app=backstage-"+crName,
		"-o", "jsonpath={.items[*].status}",
		"-n", ns,
	) // #nosec G204
	status, err := Run(cmd)
	fmt.Fprintln(GinkgoWriter, string(status))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(status)).Should(ContainSubstring(fmt.Sprintf(`"phase":%q`, expectedStatus)),
		fmt.Sprintf("backstage pod in %s status", status))
}

func VerifyBackstageCRStatus(g Gomega, ns string, crName string, expectedStatus string) {
	cmd := exec.Command(GetPlatformTool(), "get", "backstage", crName, "-o", "jsonpath={.status.conditions}", "-n", ns) // #nosec G204
	status, err := Run(cmd)
	fmt.Fprintln(GinkgoWriter, string(status))
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(status)).Should(ContainSubstring(expectedStatus),
		fmt.Sprintf("status condition with type %s should be set", expectedStatus))
}

func PatchBackstageCR(ns string, crName string, jsonPatch string, patchType string) error {
	p := patchType
	if p == "" {
		p = "strategic"
	}
	_, err := Run(exec.Command(GetPlatformTool(), "-n", ns, "patch", "backstage", crName, "--patch", jsonPatch, "--type="+p)) // #nosec G204
	return err
}

func DoesBackstageRouteExist(ns string, crName string) (bool, error) {
	routeName := "backstage-" + crName
	out, err := Run(exec.Command(GetPlatformTool(), "get", "route", routeName, "-n", ns)) // #nosec G204
	if err != nil {
		if strings.Contains(string(out), fmt.Sprintf("%q not found", routeName)) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func GetBackstageRouteHost(ns string, crName string) (string, error) {
	routeName := "backstage-" + crName

	hostBytes, err := Run(exec.Command(
		GetPlatformTool(), "get", "route", routeName, "-o", "go-template={{if .spec.host}}{{.spec.host}}{{end}}", "-n", ns)) // #nosec G204
	if err != nil {
		return "", fmt.Errorf("unable to determine host for route %s/%s: %w", ns, routeName, err)
	}
	host := string(hostBytes)
	if host != "" {
		return host, nil
	}

	// try with subdomain in case it was set
	subDomainBytes, err := Run(exec.Command(
		GetPlatformTool(), "get", "route", routeName, "-o", "go-template={{if .spec.subdomain}}{{.spec.subdomain}}{{end}}", "-n", ns)) // #nosec G204
	if err != nil {
		return "", fmt.Errorf("unable to determine subdomain for route %s/%s: %w", ns, routeName, err)
	}
	subDomain := string(subDomainBytes)
	if subDomain == "" {
		return "", nil
	}
	ingressDomainBytes, err := Run(exec.Command(GetPlatformTool(), "get", "ingresses.config/cluster", "-o", "jsonpath={.spec.domain}")) // #nosec G204
	if err != nil {
		return "", fmt.Errorf("unable to determine ingress sub-domain: %w", err)
	}
	ingressDomain := string(ingressDomainBytes)
	if ingressDomain == "" {
		return "", nil
	}
	return fmt.Sprintf("%s.%s", subDomain, ingressDomain), err
}

var defaultApiEndpointTests = []ApiEndpointTest{
	{
		Endpoint:               "/",
		ExpectedHttpStatusCode: 200,
		BodyMatcher:            ContainSubstring("You need to enable JavaScript to run this app"),
	},
	{
		Endpoint:               "/api/dynamic-plugins-info/loaded-plugins",
		ExpectedHttpStatusCode: 200,
		BodyMatcher: SatisfyAll(
			ContainSubstring("@janus-idp/backstage-scaffolder-backend-module-quay-dynamic"),
			ContainSubstring("@janus-idp/backstage-scaffolder-backend-module-regex-dynamic"),
			ContainSubstring("roadiehq-scaffolder-backend-module-utils-dynamic"),
		),
	},
}

func VerifyBackstageRoute(g Gomega, ns string, crName string, tests []ApiEndpointTest) {
	host, err := GetBackstageRouteHost(ns, crName)
	fmt.Fprintln(GinkgoWriter, host)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(host).ShouldNot(BeEmpty())

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // #nosec G402 -- test code only, not used in production
		},
	}
	httpClient := &http.Client{Transport: tr}

	performTest := func(tt ApiEndpointTest) {
		url := fmt.Sprintf("https://%s/%s", host, strings.TrimPrefix(tt.Endpoint, "/"))
		resp, rErr := httpClient.Get(url)
		g.Expect(rErr).ShouldNot(HaveOccurred(), fmt.Sprintf("error while trying to GET %q", url))
		defer resp.Body.Close()

		g.Expect(resp.StatusCode).Should(Equal(tt.ExpectedHttpStatusCode), "context: "+tt.Endpoint)
		body, rErr := io.ReadAll(resp.Body)
		g.Expect(rErr).ShouldNot(HaveOccurred(), fmt.Sprintf("error while trying to read response body from 'GET %q'", url))
		if tt.BodyMatcher != nil {
			g.Expect(string(body)).Should(tt.BodyMatcher, "context: "+tt.Endpoint)
		}
	}
	allTests := append(defaultApiEndpointTests, tests...)
	for _, tt := range allTests {
		performTest(tt)
	}
}
