package integration_tests

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("create backstage with external configuration", func() {

	var (
		ctx context.Context
		ns  string
	)

	BeforeEach(func() {
		ctx = context.Background()
		ns = createNamespace(ctx)
	})

	AfterEach(func() {
		deleteNamespace(ctx, ns)
	})

	It("refreshes config", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		if !useExistingController {
			Skip("Skipped for not real controller")
		}

		appConfig1 := "app-config1"
		secretEnv1 := "secret-env1"

		backstageName := generateRandName("")

		conf := `
organization:
  name: "my org"
`

		generateConfigMap(ctx, k8sClient, appConfig1, ns, map[string]string{"appconfig11": conf}, nil, nil)
		generateSecret(ctx, k8sClient, secretEnv1, ns, map[string]string{"sec11": "val11"}, nil, nil)

		bs := bsv1.BackstageSpec{
			Application: &bsv1.Application{
				AppConfig: &bsv1.AppConfig{
					MountPath: "/my/mount/path",
					ConfigMaps: []bsv1.ObjectKeyRef{
						{Name: appConfig1},
					},
				},
				ExtraEnvs: &bsv1.ExtraEnvs{
					Secrets: []bsv1.ObjectKeyRef{
						{Name: secretEnv1, Key: "sec11"},
					},
				},
			},
		}

		createAndReconcileBackstage(ctx, ns, bs, backstageName)

		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())

			podList := &corev1.PodList{}
			err = k8sClient.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{model.BackstageAppLabel: utils.BackstageAppLabelValue(backstageName)})
			g.Expect(err).ShouldNot(HaveOccurred())

			g.Expect(len(podList.Items)).To(Equal(1))
			podName := podList.Items[0].Name
			out, _, err := executeRemoteCommand(ctx, ns, podName, "backstage-backend", "cat /my/mount/path/appconfig11")
			g.Expect(err).ShouldNot(HaveOccurred())
			out = strings.Replace(out, "\r", "", -1)
			g.Expect(out).To(Equal(conf))

			out, _, err = executeRemoteCommand(ctx, ns, podName, "backstage-backend", "echo $sec11")
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect("val11\r\n").To(Equal(out))

		}, 5*time.Minute, 10*time.Second).Should(Succeed(), controllerMessage())

		cm := &corev1.ConfigMap{}
		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: appConfig1}, cm)
		Expect(err).ShouldNot(HaveOccurred())

		// update appconfig11
		newData := `
organization:
  name: "another org"
`
		cm.Data = map[string]string{"appconfig11": newData}
		err = k8sClient.Update(ctx, cm)
		Expect(err).ShouldNot(HaveOccurred())

		sec := &corev1.Secret{}
		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretEnv1}, sec)
		Expect(err).ShouldNot(HaveOccurred())
		newEnv := "val22"
		sec.StringData = map[string]string{"sec11": newEnv}
		err = k8sClient.Update(ctx, sec)
		Expect(err).ShouldNot(HaveOccurred())

		Eventually(func(g Gomega) {
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: appConfig1}, cm)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(cm.Data["appconfig11"]).To(Equal(newData))

			// Pod replaced so have to re-ask
			podList := &corev1.PodList{}
			err = k8sClient.List(ctx, podList, client.InNamespace(ns), client.MatchingLabels{model.BackstageAppLabel: utils.BackstageAppLabelValue(backstageName)})
			g.Expect(err).ShouldNot(HaveOccurred())

			podName := podList.Items[0].Name
			out, _, err := executeRemoteCommand(ctx, ns, podName, "backstage-backend", "cat /my/mount/path/appconfig11")
			g.Expect(err).ShouldNot(HaveOccurred())
			// TODO nicer method to compare file content with added '\r'
			// NOTE: it does not work well on envtest, real controller needed
			g.Expect(strings.ReplaceAll(out, "\r", "")).To(Equal(newData))

			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: secretEnv1}, sec)
			g.Expect(err).ShouldNot(HaveOccurred())

			out2, _, err := executeRemoteCommand(ctx, ns, podName, "backstage-backend", "echo $sec11")
			g.Expect(err).ShouldNot(HaveOccurred())
			// NOTE: it does not work well on envtest, real controller needed
			g.Expect(fmt.Sprintf("%s%s", newEnv, "\r\n")).To(Equal(out2))

		}, 10*time.Minute, 10*time.Second).Should(Succeed(), controllerMessage())

	})

})
