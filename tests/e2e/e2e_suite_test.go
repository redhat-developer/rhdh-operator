package e2e

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"redhat-developer/red-hat-developer-hub-operator/tests/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	rhdhLatestTestMode    = "rhdh-latest"
	rhdhNextTestMode      = "rhdh-next"
	rhdhAirgapTestMode    = "rhdh-airgap"
	olmDeployTestMode     = "olm"
	defaultDeployTestMode = ""
)

var _namespace = "rhdh-operator"
var testMode = os.Getenv("BACKSTAGE_OPERATOR_TEST_MODE")

// Run E2E tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	fmt.Fprintln(GinkgoWriter, "Starting Backstage Operator suite")
	RunSpecs(t, "Backstage E2E suite")
}

func installRhdhOperator(flavor string) (podLabel string) {
	Expect(helper.IsOpenShift()).Should(BeTrue(), "install RHDH script works only on OpenShift clusters!")
	cmd := exec.Command(filepath.Join(".rhdh", "scripts", "install-rhdh-catalog-source.sh"), "--"+flavor, "--install-operator", "rhdh")
	_, err := helper.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	podLabel = "app=rhdh-operator"
	return podLabel
}

func installRhdhOperatorAirgapped() (podLabel string) {
	Expect(helper.IsOpenShift()).Should(BeTrue(), "airgap preparation script for RHDH works only on OpenShift clusters!")
	indexImg, ok := os.LookupEnv("BACKSTAGE_OPERATOR_TESTS_AIRGAP_INDEX_IMAGE")
	if !ok {
		//TODO(rm3l): find a way to pass the right OCP version and arch
		indexImg = "quay.io/rhdh/iib:latest-v4.14-x86_64"
	}
	operatorVersion, ok := os.LookupEnv("BACKSTAGE_OPERATOR_TESTS_AIRGAP_OPERATOR_VERSION")
	if !ok {
		operatorVersion = "v1.1.0"
	}
	args := []string{
		"--prod_operator_index", indexImg,
		"--prod_operator_package_name", "rhdh",
		"--prod_operator_bundle_name", "rhdh-operator",
		"--prod_operator_version", operatorVersion,
	}
	if mirrorRegistry, ok := os.LookupEnv("BACKSTAGE_OPERATOR_TESTS_AIRGAP_MIRROR_REGISTRY"); ok {
		args = append(args, "--use_existing_mirror_registry", mirrorRegistry)
	}
	cmd := exec.Command(filepath.Join(".rhdh", "scripts", "prepare-restricted-environment.sh"), args...)
	_, err := helper.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	// Create a subscription in the rhdh-operator namespace
	helper.CreateNamespace(_namespace)
	cmd = exec.Command(helper.GetPlatformTool(), "-n", _namespace, "apply", "-f", "-")
	stdin, err := cmd.StdinPipe()
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	go func() {
		defer stdin.Close()
		_, _ = io.WriteString(stdin, fmt.Sprintf(`
apiVersion: operators.coreos.com/v1alpha1
kind: Subscription
metadata:
  name: rhdh
  namespace: %s
spec:
  channel: fast
  installPlanApproval: Automatic
  name: rhdh
  source: rhdh-disconnected-install
  sourceNamespace: openshift-marketplace
`, _namespace))
	}()
	_, err = helper.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	podLabel = "app=rhdh-operator"
	return podLabel
}

func installOperatorWithMakeDeploy(withOlm bool) {
	img, err := helper.Run(exec.Command("make", "--no-print-directory", "show-img"))
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
	operatorImage := strings.TrimSpace(string(img))
	imgArg := fmt.Sprintf("IMG=%s", operatorImage)

	if os.Getenv("BACKSTAGE_OPERATOR_TESTS_BUILD_IMAGES") == "true" {
		By("building the manager(Operator) image")
		cmd := exec.Command("make", "image-build", imgArg)
		_, err = helper.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	if os.Getenv("BACKSTAGE_OPERATOR_TESTS_PUSH_IMAGES") == "true" {
		By("building the manager(Operator) image")
		cmd := exec.Command("make", "image-push", imgArg)
		_, err = helper.Run(cmd)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	plt, ok := os.LookupEnv("BACKSTAGE_OPERATOR_TESTS_PLATFORM")
	if ok {
		var localClusterImageLoader func(string) error
		switch plt {
		case "kind":
			localClusterImageLoader = helper.LoadImageToKindClusterWithName
		case "k3d":
			localClusterImageLoader = helper.LoadImageToK3dClusterWithName
		case "minikube":
			localClusterImageLoader = helper.LoadImageToMinikubeClusterWithName
		}
		Expect(localClusterImageLoader).ShouldNot(BeNil(), fmt.Sprintf("unsupported platform %q to push images to", plt))
		By("loading the the manager(Operator) image on " + plt)
		err = localClusterImageLoader(operatorImage)
		ExpectWithOffset(1, err).NotTo(HaveOccurred())
	}

	By("installing CRDs")
	cmd := exec.Command("make", "install")
	_, err = helper.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	By("deploying the controller-manager")
	deployCmd := "deploy"
	if withOlm {
		deployCmd += "-olm"
	}
	cmd = exec.Command("make", deployCmd, imgArg)
	_, err = helper.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())
}

var _ = SynchronizedBeforeSuite(func() []byte {
	//runs *only* on process #1
	fmt.Fprintln(GinkgoWriter, "isOpenshift:", helper.IsOpenShift())

	managerPodLabel := "control-plane=controller-manager"

	switch testMode {
	case rhdhLatestTestMode, rhdhNextTestMode:
		_namespace = "rhdh-operator"
		managerPodLabel = installRhdhOperator(strings.TrimPrefix(testMode, "rhdh-"))
	case rhdhAirgapTestMode:
		_namespace = "rhdh-operator"
		installRhdhOperatorAirgapped()
	case olmDeployTestMode, defaultDeployTestMode:
		helper.CreateNamespace(_namespace)
		installOperatorWithMakeDeploy(testMode == olmDeployTestMode)
	default:
		Fail("unknown test mode: " + testMode)
		return nil
	}

	By("validating that the controller-manager pod is running as expected")
	EventuallyWithOffset(1, verifyControllerUp, 5*time.Minute, time.Second).WithArguments(managerPodLabel).Should(Succeed())

	return nil
}, func(_ []byte) {
	//runs on *all* processes
})

var _ = SynchronizedAfterSuite(func() {
	//runs on *all* processes
},
	// the function below *only* on process #1
	uninstallOperator,
)

func verifyControllerUp(g Gomega, managerPodLabel string) {
	// Get pod name
	cmd := exec.Command(helper.GetPlatformTool(), "get",
		"pods", "-l", managerPodLabel,
		"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}"+
			"{{ \"\\n\" }}{{ end }}{{ end }}",
		"-n", _namespace,
	)
	podOutput, err := helper.Run(cmd)
	g.Expect(err).ShouldNot(HaveOccurred())
	podNames := helper.GetNonEmptyLines(string(podOutput))
	g.Expect(podNames).Should(HaveLen(1), fmt.Sprintf("expected 1 controller pods running, but got %d", len(podNames)))
	controllerPodName := podNames[0]
	g.Expect(controllerPodName).ShouldNot(BeEmpty())

	// Validate pod status
	cmd = exec.Command(helper.GetPlatformTool(), "get",
		"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
		"-n", _namespace,
	)
	status, err := helper.Run(cmd)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(status)).Should(Equal("Running"), fmt.Sprintf("controller pod in %s status", status))
}

func getPodLogs(ns string, label string) string {
	cmd := exec.Command(helper.GetPlatformTool(), "logs",
		"-l", label,
		"-n", ns,
	)
	output, _ := helper.Run(cmd)
	return string(output)
}

func uninstallOperator() {
	switch testMode {
	case rhdhLatestTestMode, rhdhNextTestMode, rhdhAirgapTestMode:
		uninstallRhdhOperator(testMode == rhdhAirgapTestMode)
	case olmDeployTestMode, defaultDeployTestMode:
		uninstallOperatorWithMakeUndeploy(testMode == olmDeployTestMode)
	}
	helper.DeleteNamespace(_namespace, true)
}

func uninstallRhdhOperator(withAirgap bool) {
	cmd := exec.Command(helper.GetPlatformTool(), "delete", "subscription", "rhdh", "-n", _namespace, "--ignore-not-found=true")
	_, err := helper.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	cs := "rhdh-fast"
	if withAirgap {
		cs = "rhdh-disconnected-install"
	}
	cmd = exec.Command(helper.GetPlatformTool(), "delete", "catalogsource", cs, "-n", "openshift-marketplace", "--ignore-not-found=true")
	_, err = helper.Run(cmd)
	ExpectWithOffset(1, err).NotTo(HaveOccurred())

	if withAirgap {
		helper.DeleteNamespace("airgap-helper-ns", false)
	}
}

func uninstallOperatorWithMakeUndeploy(withOlm bool) {
	By("undeploying the controller-manager")
	undeployCmd := "undeploy"
	if withOlm {
		undeployCmd += "-olm"
	}
	cmd := exec.Command("make", undeployCmd)
	_, _ = helper.Run(cmd)
}
