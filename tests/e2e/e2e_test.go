package e2e

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"github.com/onsi/gomega/gcustom"

	"github.com/redhat-developer/rhdh-operator/tests/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
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

	Describe("operator metrics", func() {

		It("should not have a kube-rbac-proxy container in the controller Pod", func() {
			if testMode != olmDeployTestMode && testMode != defaultDeployTestMode {
				Skip(fmt.Sprintf("testing operator metrics endpoint not supported for this mode: %q", testMode))
			}

			By("not creating a kube-rbac-proxy sidecar")
			Eventually(func(g Gomega) {
				cmd := exec.Command(helper.GetPlatformTool(), "get",
					"pods", "-l", "app=rhdh-operator",
					"-o", "jsonpath={.items[*].spec.containers[*].name}",
					"-n", _namespace,
				)
				containerListOutput, err := helper.Run(cmd)
				g.Expect(err).ShouldNot(HaveOccurred())
				containerListNames := helper.GetNonEmptyLines(string(containerListOutput))
				g.Expect(containerListNames).Should(HaveLen(1),
					fmt.Sprintf("expected 1 container(s) in the controller pod, but got %d", len(containerListNames)))
				containerName := containerListNames[0]
				g.Expect(containerName).ToNot(BeEmpty())
				g.Expect(containerName).ShouldNot(ContainSubstring("rbac-proxy"))
			}, 3*time.Minute, time.Second).Should(Succeed())

			By("creating a Service to access the metrics")
			Eventually(func(g Gomega) {
				cmd := exec.Command(helper.GetPlatformTool(), "get", "services",
					"-l", "app=rhdh-operator",
					"-l", "app.kubernetes.io/component=metrics",
					"-o", "jsonpath={.items[*].metadata.name}",
					"-n", _namespace,
				)
				svcListOutput, err := helper.Run(cmd)
				g.Expect(err).ShouldNot(HaveOccurred())
				svcListNames := helper.GetNonEmptyLines(string(svcListOutput))
				g.Expect(svcListNames).Should(HaveLen(1),
					fmt.Sprintf("expected 1 service exposing the controller metrics, but got %d", len(svcListNames)))
				metricsServiceName := svcListNames[0]
				g.Expect(metricsServiceName).ToNot(BeEmpty())
			}, 3*time.Minute, time.Second).Should(Succeed())
		})
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
						Endpoint:               "/api/dynamic-plugins-info/loaded-plugins",
						BearerTokenRetrievalFn: helper.GuestAuth,
						ExpectedHttpStatusCode: 200,
						BodyMatcher: gcustom.MakeMatcher(func(respBody string) (bool, error) {
							if !gjson.Valid(respBody) {
								return false, fmt.Errorf("invalid json: %q", respBody)
							}
							return gjson.Get(respBody, "#").Int() > 0, nil
						}).WithMessage("be a valid and non-empty JSON array. This is the response from the 'GET /api/dynamic-plugins-info/loaded-plugins' endpoint, using the guest user."),
					},
				},
			},
			{
				name:       "with custom DB auth secret",
				crFilePath: filepath.Join("examples", "bs-existing-secret.yaml"),
				crName:     "bs-existing-secret",
			},
			{
				name:       "extra file mounts",
				crFilePath: filepath.Join("examples", "filemounts.yaml"),
				crName:     "my-rhdh-file-mounts",
			},
			{
				name:       "raw-runtime-config",
				crFilePath: filepath.Join("examples", "raw-runtime-config.yaml"),
				crName:     "bs-raw-runtime-config",
			},
		} {
			tt := tt
			When(fmt.Sprintf("applying %s (%s)", tt.name, tt.crFilePath), func() {
				var crPath string
				var crLabel string
				BeforeEach(func() {
					crPath = filepath.Join(projectDir, tt.crFilePath)
					cmd := exec.Command(helper.GetPlatformTool(), "apply", "-f", crPath, "-n", ns)
					_, err := helper.Run(cmd)
					Expect(err).ShouldNot(HaveOccurred())
					crLabel = fmt.Sprintf("rhdh.redhat.com/app=backstage-%s", tt.crName)
				})

				It("should handle CR as expected", func() {
					By("validating that the status of the custom resource created is updated or not", func() {
						// [{"lastTransitionTime":"2025-04-09T09:02:06Z","message":"","reason":"Deployed","status":"True","type":"Deployed"}]
						Eventually(helper.VerifyBackstageCRStatus, time.Minute, 10*time.Second).
							WithArguments(ns, tt.crName, ContainSubstring(`"type":"Deployed"`)).
							Should(Succeed(), fetchOperatorAndOperandLogs(managerPodLabel, ns, crLabel))
					})

					By("validating that pod(s) status.phase=Running", func() {
						Eventually(helper.VerifyBackstagePodStatus, 10*time.Minute, 10*time.Second).
							WithArguments(ns, tt.crName, "Running").
							Should(Succeed(), fetchOperatorAndOperandLogs(managerPodLabel, ns, crLabel))
					})

					if helper.IsOpenShift() {
						if tt.isRouteDisabled {
							By("ensuring no route was created", func() {
								Consistently(func(g Gomega, crName string) {
									exists, err := helper.DoesBackstageRouteExist(ns, tt.crName)
									g.Expect(err).ShouldNot(HaveOccurred())
									g.Expect(exists).Should(BeTrue())
								}, 15*time.Second, time.Second).
									WithArguments(tt.crName).
									ShouldNot(Succeed(), fetchOperatorAndOperandLogs(managerPodLabel, ns, crLabel))
							})
						} else {
							By("ensuring the route is reachable", func() {
								ensureRouteIsReachable(ns, tt.crName, crLabel, tt.additionalApiEndpointTests)
							})
						}
					} else {
						// This is how we currently instruct users to deploy the application on vanilla K8s clusters,
						// where an Ingress resource is not created OOTB by the Operator.
						// TODO(rm3l): this is until https://issues.redhat.com/browse/RHIDP-2176 is supported.
						//   For now, we want to make sure the tests cover the same area as on OpenShift, i.e.,
						//   making sure that the application is reachable end-to-end from a user standpoint.
						if os.Getenv("BACKSTAGE_OPERATOR_TESTS_K8S_CREATE_INGRESS") == "true" {
							ingressDomain := os.Getenv("BACKSTAGE_OPERATOR_TESTS_K8S_INGRESS_DOMAIN")
							if ingressDomain == "" {
								Fail("Ingress Domain should be configured via the BACKSTAGE_OPERATOR_TESTS_K8S_INGRESS_DOMAIN env var")
							}
							ingressHost := fmt.Sprintf("%s.%s", tt.crName, ingressDomain)
							By("manually creating a K8s Ingress", func() {
								cmd := exec.Command(helper.GetPlatformTool(), "-n", ns, "create", "-f", "-")
								stdin, err := cmd.StdinPipe()
								ExpectWithOffset(1, err).NotTo(HaveOccurred())
								go func() {
									defer stdin.Close()
									_, _ = io.WriteString(stdin, fmt.Sprintf(`
apiVersion: networking.k8s.io/v1
kind: Ingress
metadata:
  name: %[1]s
spec:
  rules:
  - host: %[2]s
    http:
      paths:
      - path: /
        pathType: Prefix
        backend:
          service:
            name: backstage-%[1]s
            port:
              name: http-backend
`, tt.crName, ingressHost))
								}()
								_, err = helper.Run(cmd)
								Expect(err).ShouldNot(HaveOccurred())
							})

							By("ensuring the application is fully reachable", func() {
								Eventually(helper.VerifyBackstageAppAccess, 8*time.Minute, time.Second).
									WithArguments(fmt.Sprintf("http://%s", ingressHost), tt.additionalApiEndpointTests).
									Should(Succeed(), fetchOperatorAndOperandLogs(managerPodLabel, ns, crLabel))
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
								ensureRouteIsReachable(ns, tt.crName, crLabel, tt.additionalApiEndpointTests)
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

func ensureRouteIsReachable(ns string, crName string, crLabel string, additionalApiEndpointTests []helper.ApiEndpointTest) {
	Eventually(helper.VerifyBackstageRoute, 5*time.Minute, time.Second).
		WithArguments(ns, crName, additionalApiEndpointTests).
		Should(Succeed(), fetchOperatorAndOperandLogs(managerPodLabel, ns, crLabel))
}
