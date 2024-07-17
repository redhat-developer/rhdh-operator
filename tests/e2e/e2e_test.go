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

package e2e

import (
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"redhat-developer/red-hat-developer-hub-operator/tests/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Backstage Operator E2E", func() {

	var (
		projectDir string
		ns         string
	)

	BeforeEach(func() {
		var err error
		projectDir, err = helper.GetProjectDir()
		Expect(err).ShouldNot(HaveOccurred())

		ns = fmt.Sprintf("e2e-test-%d-%s", GinkgoParallelProcess(), helper.RandString(5))
		helper.CreateNamespace(ns)
	})

	AfterEach(func() {
		helper.DeleteNamespace(ns, false)
	})

	Context("Examples CRs", func() {

		for _, tt := range []struct {
			name                       string
			crFilePath                 string
			crName                     string
			isRouteDisabled            bool
			additionalApiEndpointTests []helper.ApiEndpointTest
		}{
			{
				name:       "minimal with no spec",
				crFilePath: filepath.Join("examples", "bs1.yaml"),
				crName:     "bs1",
			},
			{
				name:       "specific route sub-domain",
				crFilePath: filepath.Join("examples", "bs-route.yaml"),
				crName:     "bs-route",
			},
			{
				name:            "route disabled",
				crFilePath:      filepath.Join("examples", "bs-route-disabled.yaml"),
				crName:          "bs-route-disabled",
				isRouteDisabled: true,
			},
			{
				name:       "RHDH CR with app-configs, dynamic plugins, extra files and extra-envs",
				crFilePath: filepath.Join("examples", "rhdh-cr-with-app-configs.yaml"),
				crName:     "bs-app-config",
				additionalApiEndpointTests: []helper.ApiEndpointTest{
					{
						Endpoint: "/api/dynamic-plugins-info/loaded-plugins",
						BearerTokenRetrievalFn: func(baseUrl string) (string, error) { // Authenticated endpoint that does not accept service tokens
							url := fmt.Sprintf("%s/api/auth/guest/refresh", baseUrl)
							tr := &http.Transport{
								TLSClientConfig: &tls.Config{
									InsecureSkipVerify: true, // #nosec G402 -- test code only, not used in production
								},
							}
							httpClient := &http.Client{Transport: tr}
							req, err := http.NewRequest("GET", url, nil)
							if err != nil {
								return "", fmt.Errorf("error while building request to GET %q: %w", url, err)
							}
							req.Header.Add("Accept", "application/json")
							resp, err := httpClient.Do(req)
							if err != nil {
								return "", fmt.Errorf("error while trying to GET %q: %w", url, err)
							}
							defer resp.Body.Close()
							body, err := io.ReadAll(resp.Body)
							if err != nil {
								return "", fmt.Errorf("error while trying to read response body from 'GET %q': %w", url, err)
							}
							if resp.StatusCode != 200 {
								return "", fmt.Errorf("expected status code 200, but got %d in response to 'GET %q', body: %s", resp.StatusCode, url, string(body))
							}
							var authResponse helper.BackstageAuthRefreshResponse
							err = json.Unmarshal(body, &authResponse)
							if err != nil {
								return "", fmt.Errorf("error while trying to decode response body from 'GET %q': %w", url, err)
							}
							return authResponse.BackstageIdentity.Token, nil
						},
						ExpectedHttpStatusCode: 200,
						BodyMatcher: SatisfyAll(
							ContainSubstring("@janus-idp/backstage-scaffolder-backend-module-quay-dynamic"),
							ContainSubstring("@janus-idp/backstage-scaffolder-backend-module-regex-dynamic"),
							ContainSubstring("roadiehq-scaffolder-backend-module-utils-dynamic"),
							ContainSubstring("backstage-plugin-catalog-backend-module-github-dynamic"),
							ContainSubstring("backstage-plugin-techdocs-backend-dynamic"),
							ContainSubstring("backstage-plugin-catalog-backend-module-gitlab-dynamic")),
					},
				},
			},
			{
				name:       "with custom DB auth secret",
				crFilePath: filepath.Join("examples", "bs-existing-secret.yaml"),
				crName:     "bs-existing-secret",
			},
		} {
			tt := tt
			When(fmt.Sprintf("applying %s (%s)", tt.name, tt.crFilePath), func() {
				var crPath string
				BeforeEach(func() {
					crPath = filepath.Join(projectDir, tt.crFilePath)
					cmd := exec.Command(helper.GetPlatformTool(), "apply", "-f", crPath, "-n", ns)
					_, err := helper.Run(cmd)
					Expect(err).ShouldNot(HaveOccurred())
				})

				It("should handle CR as expected", func() {
					By("validating that the status of the custom resource created is updated or not", func() {
						Eventually(helper.VerifyBackstageCRStatus, time.Minute, time.Second).
							WithArguments(ns, tt.crName, "Deployed").
							Should(Succeed())
					})

					By("validating that pod(s) status.phase=Running", func() {
						Eventually(helper.VerifyBackstagePodStatus, 7*time.Minute, time.Second).
							WithArguments(ns, tt.crName, "Running").
							Should(Succeed())
					})

					if helper.IsOpenShift() {
						if tt.isRouteDisabled {
							By("ensuring no route was created", func() {
								Consistently(func(g Gomega, crName string) {
									exists, err := helper.DoesBackstageRouteExist(ns, tt.crName)
									g.Expect(err).ShouldNot(HaveOccurred())
									g.Expect(exists).Should(BeTrue())
								}, 15*time.Second, time.Second).WithArguments(tt.crName).ShouldNot(Succeed())
							})
						} else {
							By("ensuring the route is reachable", func() {
								ensureRouteIsReachable(ns, tt.crName, tt.additionalApiEndpointTests)
							})
						}
					}

					var isRouteEnabledNow bool
					By("updating route spec in CR", func() {
						// enables route that was previously disabled, and disables route that was previously enabled.
						isRouteEnabledNow = tt.isRouteDisabled
						err := helper.PatchBackstageCR(ns, tt.crName, fmt.Sprintf(`
{
  "spec": {
  	"application": {
		"route": {
			"enabled": %s
		}
	}
  }
}`, strconv.FormatBool(isRouteEnabledNow)),
							"merge")
						Expect(err).ShouldNot(HaveOccurred())
					})
					if helper.IsOpenShift() {
						if isRouteEnabledNow {
							By("ensuring the route is reachable", func() {
								ensureRouteIsReachable(ns, tt.crName, tt.additionalApiEndpointTests)
							})
						} else {
							By("ensuring route no longer exists eventually", func() {
								Eventually(func(g Gomega, crName string) {
									exists, err := helper.DoesBackstageRouteExist(ns, tt.crName)
									g.Expect(err).ShouldNot(HaveOccurred())
									g.Expect(exists).Should(BeFalse())
								}, time.Minute, time.Second).WithArguments(tt.crName).Should(Succeed())
							})
						}
					}

					By("deleting CR", func() {
						cmd := exec.Command(helper.GetPlatformTool(), "delete", "-f", crPath, "-n", ns)
						_, err := helper.Run(cmd)
						Expect(err).ShouldNot(HaveOccurred())
					})

					if helper.IsOpenShift() && isRouteEnabledNow {
						By("ensuring application is no longer reachable", func() {
							Eventually(func(g Gomega, crName string) {
								exists, err := helper.DoesBackstageRouteExist(ns, tt.crName)
								g.Expect(err).ShouldNot(HaveOccurred())
								g.Expect(exists).Should(BeFalse())
							}, time.Minute, time.Second).WithArguments(tt.crName).Should(Succeed())
						})
					}
				})
			})
		}
	})
})

func ensureRouteIsReachable(ns string, crName string, additionalApiEndpointTests []helper.ApiEndpointTest) {
	Eventually(helper.VerifyBackstageRoute, 5*time.Minute, time.Second).
		WithArguments(ns, crName, additionalApiEndpointTests).
		Should(Succeed())
}
