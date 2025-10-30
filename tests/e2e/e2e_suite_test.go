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

	"github.com/redhat-developer/rhdh-operator/tests/helper"

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

var _start time.Time
var _namespace = "rhdh-operator"
var testMode = os.Getenv("BACKSTAGE_OPERATOR_TEST_MODE")
var managerPodLabel = "app=rhdh-operator"

// Run E2E tests using the Ginkgo runner.
func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	_, _ = fmt.Fprintln(GinkgoWriter, "Starting Backstage Operator suite")
	RunSpecs(t, "Backstage E2E suite")
}

func installRhdhOperatorManifest(operatorManifest string) {
	p, dErr := helper.DownloadFile(operatorManifest)
	Expect(dErr).ShouldNot(HaveOccurred())
	defer func() {
		if err := os.Remove(p); err != nil {
			_, _ = fmt.Fprintf(GinkgoWriter, "Warning: failed to remove temporary file %s: %v\n", p, err)
		}
	}()
	_, _ = fmt.Fprintf(GinkgoWriter, "Installing RHDH Operator Manifest: %q\n", p)

	cmd := exec.Command(helper.GetPlatformTool(), "apply", "-f", p)
	_, err := helper.Run(cmd)
	Expect(err).ShouldNot(HaveOccurred())
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
		defer func() { _ = stdin.Close() }()
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
	_start = time.Now()
	_, _ = fmt.Fprintln(GinkgoWriter, "isOpenshift:", helper.IsOpenShift())

	if operatorManifest := os.Getenv("OPERATOR_MANIFEST"); operatorManifest != "" {
		installRhdhOperatorManifest(operatorManifest)
	} else {
		switch testMode {
		case rhdhLatestTestMode, rhdhNextTestMode:
			managerPodLabel = installRhdhOperator(strings.TrimPrefix(testMode, "rhdh-"))
		case rhdhAirgapTestMode:
			installRhdhOperatorAirgapped()
		case olmDeployTestMode, defaultDeployTestMode:
			helper.CreateNamespace(_namespace)
			installOperatorWithMakeDeploy(testMode == olmDeployTestMode)
		default:
			Fail("unknown test mode: " + testMode)
			return nil
		}
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
	func() {
		defer uninstallOperator()
		fmt.Println(fetchOperatorLogs(managerPodLabel, false)())
	},
)

func getControllerPodName() (string, error) {
	cmd := exec.Command(helper.GetPlatformTool(), "get",
		"pods", "-l", managerPodLabel,
		"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}"+
			"{{ \"\\n\" }}{{ end }}{{ end }}",
		"-n", _namespace,
	)
	podOutput, err := helper.Run(cmd)
	if err != nil {
		return "", err
	}
	podNames := helper.GetNonEmptyLines(string(podOutput))
	if len(podNames) == 0 {
		return "", fmt.Errorf("no pods found")
	}
	return podNames[0], nil
}

func verifyControllerUp(g Gomega, managerPodLabel string) {
	controllerPodName, err := getControllerPodName()
	g.Expect(err).ShouldNot(HaveOccurred())

	// Validate pod status
	cmd := exec.Command(helper.GetPlatformTool(), "get",
		"pods", controllerPodName, "-o", "jsonpath={.status.phase}",
		"-n", _namespace,
	)
	status, err := helper.Run(cmd)
	g.Expect(err).ShouldNot(HaveOccurred())
	g.Expect(string(status)).Should(Equal("Running"), fmt.Sprintf("controller pod in %s status", status))
}

func getPodLogs(ns string, podName string, label string) string {
	opts := []string{
		"-n", ns,
		"logs",
		"--all-containers",
		"--since-time", _start.Format(time.RFC3339),
	}
	if podName != "" {
		opts = append(opts, podName)
	} else if label != "" {
		opts = append(opts, "-l", label)
	}
	cmd := exec.Command(helper.GetPlatformTool(), opts...)
	output, _ := helper.Run(cmd)
	return string(output)
}

func describePod(ns string, label string) string {
	cmd := exec.Command(helper.GetPlatformTool(), "describe", "pods", "-l", label, "-n", ns)
	output, _ := helper.Run(cmd)
	return string(output)
}

func fetchOperatorLogs(managerPodLabel string, raw bool) func() string {
	return func() string {
		var logs string
		controllerPodName, err := getControllerPodName()
		if err != nil {
			logs = fmt.Sprintf("Failed to get controller pod name: %v", err)
		} else {
			logs = getPodLogs(_namespace, controllerPodName, managerPodLabel)
		}
		if raw {
			return logs
		}
		return fmt.Sprintf("=== Operator logs ===\n%s\n", logs)
	}
}

func fetchOperandLogs(ns string, crLabel string, raw bool) func() string {
	return func() string {
		logs := getPodLogs(ns, "", crLabel)
		if raw {
			return logs
		}
		return fmt.Sprintf("=== Operand logs ===\n%s\n", logs)
	}
}

func fetchOperatorAndOperandLogs(managerPodLabel string, ns string, crLabel string) string {
	return fmt.Sprintf("%s\n\n%s\n",
		fetchOperatorLogs(managerPodLabel, false)(),
		fetchOperandLogs(ns, crLabel, false)())
}

func describeOperatorAndOperatorPods(managerPodLabel string, ns string, crLabel string) string {
	return fmt.Sprintf("%s\n\n%s\n",
		describePod(_namespace, managerPodLabel),
		describePod(ns, crLabel))
}

func uninstallOperator() {
	if operatorManifest := os.Getenv("OPERATOR_MANIFEST"); operatorManifest != "" {
		cmd := exec.Command(helper.GetPlatformTool(), "delete", "-f", operatorManifest)
		_, _ = helper.Run(cmd)
	} else {
		switch testMode {
		case rhdhLatestTestMode, rhdhNextTestMode, rhdhAirgapTestMode:
			uninstallRhdhOperator(testMode == rhdhAirgapTestMode)
		case olmDeployTestMode, defaultDeployTestMode:
			uninstallOperatorWithMakeUndeploy(testMode == olmDeployTestMode)
		}
		helper.DeleteNamespace(_namespace, true)
	}
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
