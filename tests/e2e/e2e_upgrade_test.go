package e2e

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"time"

	"github.com/redhat-developer/rhdh-operator/tests/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Operator upgrade with existing instances", func() {

	var ns string

	BeforeEach(func() {
		ns = fmt.Sprintf("e2e-test-%d-%s", GinkgoParallelProcess(), helper.RandString(5))
		helper.CreateNamespace(ns)
	})

	AfterEach(func() {
		helper.DeleteNamespace(ns, false)
	})

	When("Previous version of operator is installed and CR is created", func() {

		const crName = "my-backstage-app"

		var fromDeploymentManifest string

		BeforeEach(func() {
			if testMode != defaultDeployTestMode {
				Skip("testing upgrades currently supported only with the default deployment mode")
			}

			// Uninstall the current version of the operator (which was installed in the SynchronizedBeforeSuite),
			// because this test needs to start from a previous version, then perform the upgrade.
			uninstallOperator()

			fromDeploymentManifest = os.Getenv("FROM_OPERATOR_MANIFEST")
			Expect(fromDeploymentManifest).NotTo(BeEmpty(), "The FROM_OPERATOR_MANIFEST env var must not be empty")

			cmd := exec.Command(helper.GetPlatformTool(), "apply", "-f", fromDeploymentManifest)
			_, err := helper.Run(cmd)
			Expect(err).ShouldNot(HaveOccurred())
			EventuallyWithOffset(1, verifyControllerUp, 5*time.Minute, time.Second).
				WithArguments("app=rhdh-operator").Should(Succeed())

			cmd = exec.Command(helper.GetPlatformTool(), "-n", ns, "apply", "-f", "-")
			stdin, err := cmd.StdinPipe()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			go func() {
				defer func() {
					if err := stdin.Close(); err != nil {
						GinkgoWriter.Println("Warning: failed to close stdin pipe:", err)
					}
				}()
				_, _ = io.WriteString(stdin, fmt.Sprintf(`
apiVersion: rhdh.redhat.com/v1alpha3
kind: Backstage
metadata:
  name: my-backstage-app
  namespace: %s
`, ns))
			}()
			_, err = helper.Run(cmd)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(helper.VerifyBackstageCRStatus, time.Minute, time.Second).
				WithArguments(ns, crName, ContainSubstring(`"type":"Deployed"`)).
				Should(Succeed())
			Eventually(helper.VerifyBackstagePodStatus, 10*time.Minute, time.Second).WithArguments(ns, crName, "Running").
				Should(Succeed())
		})

		AfterEach(func() {
			for _, m := range []string{"FROM", "TO"} {
				if manifest := os.Getenv(m + "_OPERATOR_MANIFEST"); manifest != "" {
					cmd := exec.Command(helper.GetPlatformTool(), "delete", "-f", manifest, "--ignore-not-found=true")
					_, _ = helper.Run(cmd)
				}
			}
			uninstallOperator()

			if fromDeploymentManifest != "" {
				cmd := exec.Command(helper.GetPlatformTool(), "delete", "-f", fromDeploymentManifest, "--ignore-not-found=true")
				_, _ = helper.Run(cmd)
			}
		})

		It("should successfully reconcile existing CR when upgrading the operator", func() {
			By("Upgrading the operator", func() {
				if toOperatorManifest := os.Getenv("TO_OPERATOR_MANIFEST"); toOperatorManifest != "" {
					cmd := exec.Command(helper.GetPlatformTool(), "apply", "-f", toOperatorManifest)
					_, err := helper.Run(cmd)
					Expect(err).ShouldNot(HaveOccurred())
				} else {
					installOperatorWithMakeDeploy(false)
				}
				EventuallyWithOffset(1, verifyControllerUp, 5*time.Minute, 10*time.Second).
					WithArguments(managerPodLabel).Should(Succeed())
			})

			crLabel := fmt.Sprintf("rhdh.redhat.com/app=backstage-%s", crName)

			// TODO(rm3l): this might never work because the Deployment may not necessarily
			// change after an upgrade of the Operator. It might not result in a different replicas
			// if the newer operator did not change anything.
			// By("ensuring the current operator eventually reconciled through the new ReplicaSet")
			// Eventually(func(g Gomega) {
			//	cmd := exec.Command(helper.GetPlatformTool(), "get",
			//		"replicasets", "-l", crLabel,
			//		"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}"+
			//			"{{ \"\\n\" }}{{ end }}{{ end }}",
			//		"-n", ns,
			//	)
			//	rsOutput, err := helper.Run(cmd)
			//	g.Expect(err).ShouldNot(HaveOccurred())
			//	rsNames := helper.GetNonEmptyLines(string(rsOutput))
			//	g.Expect(len(rsNames)).Should(BeNumerically(">=", 2),
			//		fmt.Sprintf("expected at least 2 Backstage operand ReplicaSets, but got %d", len(rsNames)))
			// }, 3*time.Minute, 3*time.Second).Should(Succeed(), fetchOperatorLogs)

			By("checking the status of the existing CR")
			// [{"lastTransitionTime":"2025-04-09T09:02:06Z","message":"","reason":"Deployed","status":"True","type":"Deployed"}]
			Eventually(helper.VerifyBackstageCRStatus, 15*time.Minute, 10*time.Second).
				WithArguments(ns, crName, ContainSubstring(`"reason":"Deployed"`)).
				Should(Succeed(), func() string {
					return fmt.Sprintf("%s\n---\n%s",
						fetchOperatorAndOperandLogs(managerPodLabel, ns, crLabel),
						describeOperatorAndOperatorPods(managerPodLabel, ns, crLabel),
					)
				})

			By("checking the Backstage operand pod")
			Eventually(func(g Gomega) {
				// Get pod name
				cmd := exec.Command(helper.GetPlatformTool(), "get",
					"pods", "-l", crLabel,
					"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", ns,
				)
				podOutput, err := helper.Run(cmd)
				g.Expect(err).ShouldNot(HaveOccurred())
				podNames := helper.GetNonEmptyLines(string(podOutput))
				g.Expect(podNames).Should(
					HaveLen(1), fmt.Sprintf("expected 1 Backstage operand pod(s) running, but got %d", len(podNames)))
			}, 15*time.Minute, 10*time.Second).Should(Succeed(), func() string {
				return fmt.Sprintf("%s\n---\n%s",
					fetchOperatorAndOperandLogs(managerPodLabel, ns, crLabel),
					describeOperatorAndOperatorPods(managerPodLabel, ns, crLabel),
				)
			})
		})
	})

})
