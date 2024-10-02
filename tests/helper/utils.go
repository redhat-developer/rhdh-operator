package helper

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/util/rand"
	"k8s.io/client-go/discovery"
	ctrl "sigs.k8s.io/controller-runtime"
)

var letterRunes = []rune("abcdefghijklmnopqrstuvwxyz0123456789")

var (
	_isOpenShift bool
)

func init() {
	_isOpenShift = func() bool {
		restConfig := ctrl.GetConfigOrDie()
		dcl, err := discovery.NewDiscoveryClientForConfig(restConfig)
		if err != nil {
			return false
		}

		apiList, err := dcl.ServerGroups()
		if err != nil {
			return false
		}

		apiGroups := apiList.Groups
		for i := 0; i < len(apiGroups); i++ {
			if apiGroups[i].Name == "route.openshift.io" {
				return true
			}
		}

		return false
	}()
}

func GetPlatformTool() string {
	if IsOpenShift() {
		return "oc"
	}
	return "kubectl"
}

func saveImageArchive(name string) (string, error) {
	cEng, err := Run(exec.Command("make", "--no-print-directory", "show-container-engine"))
	if err != nil {
		return "", err
	}
	containerEngine := strings.TrimSpace(string(cEng))

	// check if image exists locally first. It not, try to pull it
	_, err = Run(exec.Command(containerEngine, "image", "inspect", name)) // #nosec G204
	if err != nil {
		// image likely does not exist locally
		_, err = Run(exec.Command(containerEngine, "image", "pull", name)) // #nosec G204
		if err != nil {
			return "", fmt.Errorf("image %q not found locally and not able to pull it: %w", name, err)
		}
	}

	f, err := os.CreateTemp("", "tmp_image_archive-")
	if err != nil {
		return "", err
	}
	tmp := f.Name()
	_, err = Run(exec.Command(containerEngine, "image", "save", "--output", tmp, name)) // #nosec G204
	return tmp, err
}

// LoadImageToKindClusterWithName loads a local container image to the kind cluster
func LoadImageToKindClusterWithName(name string) error {
	archive, err := saveImageArchive(name)
	defer func() {
		if archive != "" {
			_ = os.Remove(archive)
		}
	}()
	if err != nil {
		return err
	}

	cluster := "kind"
	if v, ok := os.LookupEnv("BACKSTAGE_OPERATOR_TESTS_KIND_CLUSTER"); ok {
		cluster = v
	}
	cmd := exec.Command("kind", "load", "image-archive", "--name", cluster, archive) // #nosec G204
	_, err = Run(cmd)
	return err
}

// LoadImageToK3dClusterWithName loads a local container image to the k3d cluster
func LoadImageToK3dClusterWithName(name string) error {
	archive, err := saveImageArchive(name)
	defer func() {
		if archive != "" {
			_ = os.Remove(archive)
		}
	}()
	if err != nil {
		return err
	}

	cluster := "k3s-default"
	if v, ok := os.LookupEnv("BACKSTAGE_OPERATOR_TESTS_K3D_CLUSTER"); ok {
		cluster = v
	}
	cmd := exec.Command("k3d", "image", "import", archive, "--cluster", cluster) // #nosec G204
	_, err = Run(cmd)
	return err
}

// LoadImageToMinikubeClusterWithName loads a local container image to the Minikube cluster
func LoadImageToMinikubeClusterWithName(name string) error {
	archive, err := saveImageArchive(name)
	defer func() {
		if archive != "" {
			_ = os.Remove(archive)
		}
	}()
	if err != nil {
		return err
	}

	_, err = Run(exec.Command("minikube", "image", "load", archive)) // #nosec G204
	return err
}

// GetNonEmptyLines converts given command output string into individual objects
// according to line breakers, and ignores the empty elements in it.
func GetNonEmptyLines(output string) []string {
	var res []string
	elements := strings.Split(output, "\n")
	for _, element := range elements {
		if element != "" {
			res = append(res, element)
		}
	}

	return res
}

// Run executes the provided command within this context
func Run(cmd *exec.Cmd) ([]byte, error) {
	dir, _ := GetProjectDir()
	cmd.Dir = dir
	fmt.Fprintf(GinkgoWriter, "running dir: %s\n", cmd.Dir)

	cmd.Env = append(cmd.Env, os.Environ()...)

	if err := os.Chdir(cmd.Dir); err != nil {
		fmt.Fprintf(GinkgoWriter, "chdir dir: %s\n", err)
	}

	command := strings.Join(cmd.Args, " ")
	fmt.Fprintf(GinkgoWriter, "running: %s\n", command)

	var stdBuffer bytes.Buffer
	mw := io.MultiWriter(GinkgoWriter, &stdBuffer)
	cmd.Stdout = mw
	cmd.Stderr = mw

	err := cmd.Run()
	outBytes := stdBuffer.Bytes()
	if err != nil {
		return outBytes, fmt.Errorf("%s failed with error: (%v) %s", command, err, string(outBytes))
	}

	return outBytes, nil
}

// GetProjectDir will return the directory where the project is
func GetProjectDir() (string, error) {
	wd, err := os.Getwd()
	if err != nil {
		return wd, err
	}
	wd = strings.Replace(wd, "/tests/e2e", "", -1)
	return wd, nil
}

func CreateNamespace(ns string) {
	cmd := exec.Command(GetPlatformTool(), "create", "namespace", ns) // #nosec G204
	out, err := Run(cmd)
	if err != nil && strings.Contains(string(out), fmt.Sprintf("%q already exists", ns)) {
		return
	}
	Expect(err).ShouldNot(HaveOccurred())
}

func DeleteNamespace(ns string, wait bool) {
	cmd := exec.Command(GetPlatformTool(),
		"delete",
		"namespace",
		ns,
		fmt.Sprintf("--wait=%s", strconv.FormatBool(wait)),
		"--ignore-not-found=true",
	) // #nosec G204
	_, err := Run(cmd)
	Expect(err).ShouldNot(HaveOccurred())
}

func RandString(n int) string {
	b := make([]rune, n)
	for i := range b {
		b[i] = letterRunes[rand.Intn(len(letterRunes))]
	}
	return string(b)
}

func IsOpenShift() bool {
	return _isOpenShift
}
