package e2e

import (
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/redhat-developer/rhdh-operator/tests/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("Operator upgrade with existing instances", func() {

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

	When("Previous version of operator is installed and CR is created", func() {

		const managerPodLabel = "control-plane=controller-manager"
		const crName = "my-backstage-app"

		var fromDeploymentManifest = filepath.Join(projectDir, "tests", "e2e", "testdata", "rhdh-operator-1.2.yaml")

		BeforeEach(func() {
			if testMode != defaultDeployTestMode {
				Skip("testing upgrades currently supported only with the default deployment mode")
			}

			// Uninstall the current version of the operator (which was installed in the SynchronizedBeforeSuite),
			// because this test needs to start from a previous version, then perform the upgrade.
			uninstallOperator()

			cmd := exec.Command(helper.GetPlatformTool(), "apply", "-f", fromDeploymentManifest)
			_, err := helper.Run(cmd)
			Expect(err).ShouldNot(HaveOccurred())
			EventuallyWithOffset(1, verifyControllerUp, 5*time.Minute, time.Second).WithArguments(managerPodLabel).Should(Succeed())

			cmd = exec.Command(helper.GetPlatformTool(), "-n", ns, "create", "-f", "-")
			stdin, err := cmd.StdinPipe()
			ExpectWithOffset(1, err).NotTo(HaveOccurred())
			go func() {
				defer stdin.Close()
				_, _ = io.WriteString(stdin, fmt.Sprintf(`
apiVersion: rhdh.redhat.com/v1alpha1
kind: Backstage
metadata:
  name: my-backstage-app
  namespace: %s
`, ns))
			}()
			_, err = helper.Run(cmd)
			Expect(err).ShouldNot(HaveOccurred())

			Eventually(helper.VerifyBackstageCRStatus, time.Minute, time.Second).WithArguments(ns, crName, `"reason":"Deployed"`).Should(Succeed())
		})

		AfterEach(func() {
			uninstallOperator()

			cmd := exec.Command(helper.GetPlatformTool(), "delete", "-f", fromDeploymentManifest, "--ignore-not-found=true")
			_, err := helper.Run(cmd)
			Expect(err).ShouldNot(HaveOccurred())
		})

		It("should successfully reconcile existing CR when upgrading the operator", func() {
			By("Upgrading the operator", func() {
				installOperatorWithMakeDeploy(false)
				EventuallyWithOffset(1, verifyControllerUp, 5*time.Minute, 3*time.Second).WithArguments(managerPodLabel).Should(Succeed())
			})

			fetchOperatorLogs := func() string {
				return fmt.Sprintf("=== Operator logs ===\n%s\n", getPodLogs(_namespace, managerPodLabel))
			}

			crLabel := fmt.Sprintf("rhdh.redhat.com/app=backstage-%s", crName)

			By("ensuring the current operator eventually reconciled through the creation of a new ReplicaSet of the application")
			Eventually(func(g Gomega) {
				cmd := exec.Command(helper.GetPlatformTool(), "get",
					"replicasets", "-l", crLabel,
					"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}"+
						"{{ \"\\n\" }}{{ end }}{{ end }}",
					"-n", ns,
				)
				rsOutput, err := helper.Run(cmd)
				g.Expect(err).ShouldNot(HaveOccurred())
				rsNames := helper.GetNonEmptyLines(string(rsOutput))
				g.Expect(len(rsNames)).Should(BeNumerically(">=", 2),
					fmt.Sprintf("expected at least 2 Backstage operand ReplicaSets, but got %d", len(rsNames)))
			}, 3*time.Minute, 3*time.Second).Should(Succeed(), fetchOperatorLogs)

			By("checking the status of the existing CR")
			Eventually(helper.VerifyBackstageCRStatus, 5*time.Minute, 3*time.Second).WithArguments(ns, crName, `"reason":"Deployed"`).
				Should(Succeed(), func() string {
					return fmt.Sprintf("=== Operator logs ===\n%s\n", getPodLogs(_namespace, managerPodLabel))
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
				g.Expect(podNames).Should(HaveLen(1), fmt.Sprintf("expected 1 Backstage operand pod(s) running, but got %d", len(podNames)))
			}, 10*time.Minute, 5*time.Second).Should(Succeed(), func() string {
				return fmt.Sprintf("=== Operand logs ===\n%s\n", getPodLogs(ns, crLabel))
			})
		})
	})

})
