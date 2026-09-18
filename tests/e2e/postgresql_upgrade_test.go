package e2e

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"time"

	"github.com/redhat-developer/rhdh-operator/tests/helper"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

const (
	postgresqlSourceImage   = "postgresql-15"
	postgresqlSourceVersion = "15."
	postgresqlTargetImage   = "postgresql-18"
	postgresqlTargetVersion = "18."
	postgresqlProofValue    = "postgresql-15-to-18"
)

type postgresqlUpgradeState struct {
	dumpPath        string
	postgresPodUID  string
	pvcUID          string
	secretUID       string
	backstagePodUID string
	databaseNames   []string
}

func preparePostgresqlUpgrade(namespace, crName string) *postgresqlUpgradeState {
	state := &postgresqlUpgradeState{}
	postgresPod := waitForPostgresql(namespace, crName, postgresqlSourceImage, postgresqlSourceVersion)

	By("seeding data that must survive the PostgreSQL upgrade")
	_, err := runPostgresqlSQL(namespace, postgresPod, `
CREATE TABLE public.rhdh_postgresql_upgrade_test (marker text PRIMARY KEY);
INSERT INTO public.rhdh_postgresql_upgrade_test VALUES ('`+postgresqlProofValue+`');`)
	Expect(err).NotTo(HaveOccurred())
	Expect(queryPostgresqlUpgradeProof(namespace, postgresPod)).To(Equal(postgresqlProofValue))

	state.postgresPodUID = resourceUID(namespace, "pod", postgresPod)
	state.pvcUID = resourceUID(namespace, "pvc", postgresqlPVCName(crName))
	state.secretUID = resourceUID(namespace, "secret", postgresqlSecretName(crName))
	state.backstagePodUID = readyPodUID(namespace, fmt.Sprintf("rhdh.redhat.com/app=backstage-%s", crName))
	state.databaseNames = postgresqlDatabaseNames(namespace, postgresPod)

	By("stopping Backstage before taking the PostgreSQL dump")
	setBackstageReplicas(namespace, crName, 0)
	waitForNoPods(namespace, fmt.Sprintf("rhdh.redhat.com/app=backstage-%s", crName))

	By("dumping all PostgreSQL 15 databases and roles")
	state.dumpPath = dumpPostgresql(namespace, postgresPod)

	return state
}

func completePostgresqlUpgrade(namespace, crName string, state *postgresqlUpgradeState) {
	statefulSetName := postgresqlStatefulSetName(crName)

	By("waiting for the upgraded operator to select the PostgreSQL 18 image")
	Eventually(func(g Gomega) {
		out, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "get", "statefulset",
			statefulSetName, "-o", "jsonpath={.spec.template.spec.containers[0].image}"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(out))).To(ContainSubstring(postgresqlTargetImage))
	}, 5*time.Minute, 5*time.Second).Should(Succeed())

	operatorScaledDown := false
	DeferCleanup(func() {
		if operatorScaledDown {
			if err := scaleDeploymentCommand(_namespace, "rhdh-operator", 1); err != nil {
				GinkgoWriter.Printf("Warning: failed to restore the operator replica: %v\n", err)
			}
		}
	})

	By("stopping the operator while replacing the PostgreSQL volume")
	scaleDeployment(_namespace, "rhdh-operator", 0)
	operatorScaledDown = true
	waitForNoPods(_namespace, managerPodLabel)

	By("deleting the PostgreSQL 15 StatefulSet and persistent volume")
	deleteResource(namespace, "statefulset", statefulSetName)
	deleteResource(namespace, "pvc", postgresqlPVCName(crName))
	Expect(resourceUID(namespace, "secret", postgresqlSecretName(crName))).To(Equal(state.secretUID))

	By("restarting the operator to provision a fresh PostgreSQL 18 database")
	scaleDeployment(_namespace, "rhdh-operator", 1)
	operatorScaledDown = false
	waitForOperatorRollout()
	EventuallyWithOffset(1, verifyControllerUp, 5*time.Minute, 5*time.Second).
		WithArguments(managerPodLabel).Should(Succeed())

	postgresPod := waitForPostgresql(namespace, crName, postgresqlTargetImage, postgresqlTargetVersion)
	Expect(resourceUID(namespace, "pod", postgresPod)).NotTo(Equal(state.postgresPodUID))
	Expect(resourceUID(namespace, "pvc", postgresqlPVCName(crName))).NotTo(Equal(state.pvcUID))
	Expect(resourceUID(namespace, "secret", postgresqlSecretName(crName))).To(Equal(state.secretUID))

	By("restoring the PostgreSQL 15 dump into PostgreSQL 18")
	restorePostgresql(namespace, postgresPod, state.dumpPath)
	Expect(postgresqlDatabaseNames(namespace, postgresPod)).To(Equal(state.databaseNames))
	refreshPostgresqlCollations(namespace, postgresPod)
	Expect(queryPostgresqlUpgradeProof(namespace, postgresPod)).To(Equal(postgresqlProofValue))

	By("starting Backstage against the restored PostgreSQL 18 database")
	setBackstageReplicas(namespace, crName, 1)
	waitForBackstagePodReplacement(namespace, crName, state.backstagePodUID)
}

func waitForPostgresql(namespace, crName, imageSubstring, versionPrefix string) string {
	var podName string
	label := fmt.Sprintf("rhdh.redhat.com/app=%s", postgresqlStatefulSetName(crName))

	Eventually(func(g Gomega) {
		out, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "get", "pods", "-l", label,
			"-o", "go-template={{ range .items }}{{ if not .metadata.deletionTimestamp }}{{ .metadata.name }}{{ \"\\n\" }}{{ end }}{{ end }}"))
		g.Expect(err).NotTo(HaveOccurred())
		podNames := helper.GetNonEmptyLines(string(out))
		g.Expect(podNames).To(HaveLen(1))
		podName = podNames[0]

		out, err = helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "get", "pod", podName,
			"-o", `jsonpath={.status.conditions[?(@.type=="Ready")].status}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(out))).To(Equal("True"))

		out, err = helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "get", "pod", podName,
			"-o", "jsonpath={.spec.containers[0].image}"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(out))).To(ContainSubstring(imageSubstring))

		out, err = helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "exec", podName, "--",
			"psql", "-X", "-U", "postgres", "-d", "postgres", "-tA", "-c", "SHOW server_version;"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(out))).To(HavePrefix(versionPrefix))
	}, 10*time.Minute, 10*time.Second).Should(Succeed())

	return podName
}

func runPostgresqlSQL(namespace, podName, sql string) ([]byte, error) {
	return helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "exec", podName, "--",
		"psql", "-X", "-U", "postgres", "-d", "postgres", "-v", "ON_ERROR_STOP=1", "-c", sql))
}

func queryPostgresqlUpgradeProof(namespace, podName string) string {
	out, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "exec", podName, "--",
		"psql", "-X", "-U", "postgres", "-d", "postgres", "-tA", "-v", "ON_ERROR_STOP=1", "-c",
		"SELECT marker FROM public.rhdh_postgresql_upgrade_test;"))
	Expect(err).NotTo(HaveOccurred())
	return strings.TrimSpace(string(out))
}

func dumpPostgresql(namespace, podName string) string {
	dumpFile, err := os.CreateTemp("", "rhdh-postgresql-15-*.sql")
	Expect(err).NotTo(HaveOccurred())
	dumpPath := dumpFile.Name()
	DeferCleanup(func() {
		Expect(os.Remove(dumpPath)).To(Succeed())
	})

	var stderr bytes.Buffer
	cmd := exec.Command(helper.GetPlatformTool(), "-n", namespace, "exec", podName, "--", "pg_dumpall", "-U", "postgres")
	err = runCommand(cmd, nil, dumpFile, io.MultiWriter(GinkgoWriter, &stderr))
	closeErr := dumpFile.Close()
	Expect(err).NotTo(HaveOccurred(), stderr.String())
	Expect(closeErr).NotTo(HaveOccurred())

	info, err := os.Stat(dumpPath)
	Expect(err).NotTo(HaveOccurred())
	Expect(info.Size()).To(BeNumerically(">", 0))
	return dumpPath
}

func restorePostgresql(namespace, podName, dumpPath string) {
	dumpFile, err := os.Open(dumpPath)
	Expect(err).NotTo(HaveOccurred())
	defer func() {
		Expect(dumpFile.Close()).To(Succeed())
	}()

	var stdout, stderr bytes.Buffer
	cmd := exec.Command(helper.GetPlatformTool(), "-n", namespace, "exec", "-i", podName, "--",
		"psql", "-X", "-U", "postgres", "-d", "postgres", "-v", "ON_ERROR_STOP=0", "--quiet")
	err = runCommand(cmd, dumpFile, io.MultiWriter(GinkgoWriter, &stdout), io.MultiWriter(GinkgoWriter, &stderr))
	Expect(err).NotTo(HaveOccurred(), stderr.String())

	Expect(validatePostgresqlRestoreErrors(stderr.String())).To(Succeed())
}

func validatePostgresqlRestoreErrors(stderr string) error {
	errorLines := make([]string, 0)
	for _, line := range strings.Split(stderr, "\n") {
		lowerLine := strings.ToLower(line)
		if strings.Contains(line, "ERROR:") || strings.Contains(line, "FATAL:") || strings.Contains(lowerLine, "psql: error:") {
			errorLines = append(errorLines, strings.TrimSpace(line))
		}
	}
	if len(errorLines) != 1 {
		if len(errorLines) == 0 {
			return fmt.Errorf("expected one bootstrap role conflict, got none")
		}
		return fmt.Errorf("unexpected PostgreSQL restore errors: %s", strings.Join(errorLines, "; "))
	}
	if !regexp.MustCompile(`^ERROR:\s+role "postgres" already exists$`).MatchString(errorLines[0]) {
		return fmt.Errorf("unexpected PostgreSQL restore errors: %s", errorLines[0])
	}
	return nil
}

func postgresqlDatabaseNames(namespace, podName string) []string {
	out, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "exec", podName, "--",
		"psql", "-X", "-U", "postgres", "-d", "postgres", "-tA", "-v", "ON_ERROR_STOP=1", "-c",
		"SELECT datname FROM pg_database WHERE datistemplate = false ORDER BY datname;"))
	Expect(err).NotTo(HaveOccurred())
	return helper.GetNonEmptyLines(strings.TrimSpace(string(out)))
}

func refreshPostgresqlCollations(namespace, podName string) {
	out, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "exec", podName, "--",
		"psql", "-X", "-U", "postgres", "-d", "postgres", "-tA", "-v", "ON_ERROR_STOP=1", "-c",
		"SELECT format('ALTER DATABASE %I REFRESH COLLATION VERSION;', datname) FROM pg_database WHERE datallowconn AND datname <> 'template0' ORDER BY datname;"))
	Expect(err).NotTo(HaveOccurred())

	for _, statement := range helper.GetNonEmptyLines(string(out)) {
		_, err = runPostgresqlSQL(namespace, podName, statement)
		Expect(err).NotTo(HaveOccurred())
	}
}

func runCommand(cmd *exec.Cmd, stdin io.Reader, stdout, stderr io.Writer) error {
	projectDir, err := helper.GetProjectDir()
	if err != nil {
		return err
	}
	cmd.Dir = projectDir
	cmd.Env = os.Environ()
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	GinkgoWriter.Printf("running: %s\n", strings.Join(cmd.Args, " "))
	return cmd.Run()
}

func setBackstageReplicas(namespace, crName string, replicas int) {
	patch := fmt.Sprintf(`{"spec":{"deployment":{"patch":{"spec":{"replicas":%d}}}}}`, replicas)
	_, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "patch", "backstage", crName,
		"--type=merge", "--patch", patch))
	Expect(err).NotTo(HaveOccurred())
}

func scaleDeployment(namespace, name string, replicas int) {
	err := scaleDeploymentCommand(namespace, name, replicas)
	Expect(err).NotTo(HaveOccurred())
}

func scaleDeploymentCommand(namespace, name string, replicas int) error {
	_, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "scale", "deployment", name,
		fmt.Sprintf("--replicas=%d", replicas)))
	return err
}

func waitForNoPods(namespace, label string) {
	Eventually(func(g Gomega) {
		out, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "get", "pods", "-l", label,
			"-o", "name"))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(strings.TrimSpace(string(out))).To(BeEmpty())
	}, 5*time.Minute, 5*time.Second).Should(Succeed())
}

func deleteResource(namespace, kind, name string) {
	_, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "delete", kind, name,
		"--wait=true", "--timeout=5m"))
	Expect(err).NotTo(HaveOccurred())
}

func resourceUID(namespace, kind, name string) string {
	out, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "get", kind, name,
		"-o", "jsonpath={.metadata.uid}"))
	Expect(err).NotTo(HaveOccurred())
	uid := strings.TrimSpace(string(out))
	Expect(uid).NotTo(BeEmpty())
	return uid
}

func readyPodUID(namespace, label string) string {
	var uid string
	Eventually(func(g Gomega) {
		out, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "get", "pods", "-l", label,
			"-o", `jsonpath={range .items[?(@.status.phase=="Running")]}{.metadata.uid}{" "}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}`))
		g.Expect(err).NotTo(HaveOccurred())
		lines := helper.GetNonEmptyLines(string(out))
		g.Expect(lines).To(HaveLen(1))
		fields := strings.Fields(lines[0])
		g.Expect(fields).To(HaveLen(2))
		g.Expect(fields[1]).To(Equal("True"))
		uid = fields[0]
	}, 10*time.Minute, 10*time.Second).Should(Succeed())
	return uid
}

func waitForBackstagePodReplacement(namespace, crName, previousUID string) {
	label := fmt.Sprintf("rhdh.redhat.com/app=backstage-%s", crName)
	Eventually(func(g Gomega) {
		out, err := helper.Run(exec.Command(helper.GetPlatformTool(), "-n", namespace, "get", "pods", "-l", label,
			"-o", `jsonpath={range .items[*]}{.metadata.uid}{" "}{.status.phase}{" "}{.status.conditions[?(@.type=="Ready")].status}{"\n"}{end}`))
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(string(out)).NotTo(ContainSubstring(previousUID))
		lines := helper.GetNonEmptyLines(string(out))
		g.Expect(lines).To(HaveLen(1))
		fields := strings.Fields(lines[0])
		g.Expect(fields).To(HaveLen(3))
		g.Expect(fields[0]).NotTo(Equal(previousUID))
		g.Expect(fields[1]).To(Equal("Running"))
		g.Expect(fields[2]).To(Equal("True"))
	}, 15*time.Minute, 10*time.Second).Should(Succeed())
}

func postgresqlStatefulSetName(crName string) string {
	return fmt.Sprintf("backstage-psql-%s", crName)
}

func postgresqlPVCName(crName string) string {
	return fmt.Sprintf("data-%s-0", postgresqlStatefulSetName(crName))
}

func postgresqlSecretName(crName string) string {
	return fmt.Sprintf("backstage-psql-secret-%s", crName)
}
