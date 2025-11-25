package integration_tests

import (
	"context"
	"time"

	"sigs.k8s.io/yaml"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"

	corev1 "k8s.io/api/core/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha5"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("create backstage with CR configured", func() {

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

	It("creates Backstage with external configuration", func() {

		appConfig1 := generateConfigMap(ctx, k8sClient, "app-config1", ns, map[string]string{"key11": "app:", "key12": "app:"}, nil, nil)
		appConfig2 := generateConfigMap(ctx, k8sClient, "app-config2", ns, map[string]string{"key21": "app:", "key22": "app:"}, nil, nil)
		appConfig3 := generateConfigMap(ctx, k8sClient, "app-config3.dot", ns, map[string]string{"key.31": "app31:"}, nil, nil)

		cmFile1 := generateConfigMap(ctx, k8sClient, "cm-file1", ns, map[string]string{"cm11": "11", "cm12": "12"}, nil, nil)
		cmFile2 := generateConfigMap(ctx, k8sClient, "cm-file2", ns, map[string]string{"cm21": "21", "cm22": "22"}, nil, nil)
		cmFile3 := generateConfigMap(ctx, k8sClient, "cm-file3.dot", ns, map[string]string{"cm.31": "31"}, nil, nil)
		cmFileWithPath := generateConfigMap(ctx, k8sClient, "cm-file-withpath", ns, map[string]string{"cm": "withpath"}, nil, nil)

		secretFile1 := generateSecret(ctx, k8sClient, "secret-file1", ns, map[string]string{"sec11": "val11", "sec12": "val12"}, nil, nil)
		secretFile2 := generateSecret(ctx, k8sClient, "secret-file2", ns, map[string]string{"sec21": "val21", "sec22": "val22"}, nil, nil)
		secretFile3 := generateSecret(ctx, k8sClient, "secret-file3.dot", ns, map[string]string{"sec.31": "val31", "sec.32": "val22"}, nil, nil)
		secretFileWithPath := generateSecret(ctx, k8sClient, "secret-file-withpath", ns, map[string]string{"secret": "withpath"}, nil, nil)

		cmEnv1 := generateConfigMap(ctx, k8sClient, "cm-env1", ns, map[string]string{"cm11": "11", "cm12": "12"}, nil, nil)
		cmEnv2 := generateConfigMap(ctx, k8sClient, "cm-env2", ns, map[string]string{"cm21": "21", "cm22": "22"}, nil, nil)

		secretEnv1 := generateSecret(ctx, k8sClient, "secret-env1", ns, map[string]string{"sec11": "val11", "sec12": "val12"}, nil, nil)
		_ = generateSecret(ctx, k8sClient, "secret-env2", ns, map[string]string{"sec21": "val21", "sec22": "val22"}, nil, nil)

		patch, _ := yaml.YAMLToJSON([]byte(`
spec:
  template:
    spec:
      containers:
        - name: sidecar
          image: busybox
`))
		bs := bsv1.BackstageSpec{
			Deployment: &bsv1.BackstageDeployment{
				Patch: &apiextensionsv1.JSON{Raw: patch},
			},
			Application: &bsv1.Application{
				AppConfig: &bsv1.AppConfig{
					MountPath: "/my/mount/path",
					ConfigMaps: []bsv1.FileObjectRef{
						{Name: appConfig1},
						{Name: appConfig2, Key: "key21"},
						{Name: appConfig3},
					},
				},
				ExtraFiles: &bsv1.ExtraFiles{
					MountPath: "/my/file/path",
					ConfigMaps: []bsv1.FileObjectRef{
						{Name: cmFile1},
						{Name: cmFile2, Key: "cm21"},
						{Name: cmFile3, Containers: []string{"*"}},
						{Name: cmFileWithPath, MountPath: "/cm/file/withpath"},
					},
					Secrets: []bsv1.FileObjectRef{
						{Name: secretFile1, Key: "sec11"},
						{Name: secretFile2, Key: "sec21"},
						{Name: secretFile3, Key: "sec.31", Containers: []string{"sidecar"}},
						{Name: secretFileWithPath, MountPath: "/secret/file/withpath"},
					},
				},
				ExtraEnvs: &bsv1.ExtraEnvs{
					ConfigMaps: []bsv1.EnvObjectRef{
						{Name: cmEnv1},
						{Name: cmEnv2, Key: "cm21", Containers: []string{"*"}},
					},
					Secrets: []bsv1.EnvObjectRef{
						{Name: secretEnv1, Key: "sec11"},
					},
					Envs: []bsv1.Env{
						{Name: "env1", Value: "val1"},
					},
				},
			},
		}

		backstageName := createAndReconcileBackstage(ctx, ns, bs, "")

		Eventually(func(g Gomega) {
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).ShouldNot(HaveOccurred())

			podSpec := deploy.Spec.Template.Spec
			backstageContainer := podSpec.Containers[model.BackstageContainerIndex(deploy)]
			sidecarContainer := corev1.Container{}
			for i := range podSpec.Containers {
				if podSpec.Containers[i].Name == "sidecar" {
					sidecarContainer = podSpec.Containers[i]
					break
				}
			}

			By("checking if app-config volumes are added to PodSpec")
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(appConfig1)).To(BeAddedAsVolumeToPodSpec(podSpec))
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(appConfig2)).To(BeAddedAsVolumeToPodSpec(podSpec))
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(appConfig3)).To(BeAddedAsVolumeToPodSpec(podSpec))

			By("checking if app-config volumes are mounted to the Backstage container")
			g.Expect("/my/mount/path/key11").To(BeMountedToContainer(backstageContainer))
			g.Expect("/my/mount/path/key12").To(BeMountedToContainer(backstageContainer))
			g.Expect("/my/mount/path/key21").To(BeMountedToContainer(backstageContainer))
			g.Expect("/my/mount/path/key22").NotTo(BeMountedToContainer(backstageContainer))
			g.Expect("/my/mount/path/key.31").To(BeMountedToContainer(backstageContainer))

			By("checking if app-config args are added to the Backstage container")
			g.Expect("/my/mount/path/key11").To(BeAddedAsArgToContainer(backstageContainer))
			g.Expect("/my/mount/path/key12").To(BeAddedAsArgToContainer(backstageContainer))
			g.Expect("/my/mount/path/key21").To(BeAddedAsArgToContainer(backstageContainer))
			g.Expect("/my/mount/path/key22").NotTo(BeAddedAsArgToContainer(backstageContainer))
			g.Expect("/my/mount/path/key.31").To(BeAddedAsArgToContainer(backstageContainer))

			By("checking if extra-cm-file volumes are added to PodSpec")
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(cmFile1)).To(BeAddedAsVolumeToPodSpec(podSpec))
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(cmFile2)).To(BeAddedAsVolumeToPodSpec(podSpec))
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret(cmFile3)).To(BeAddedAsVolumeToPodSpec(podSpec))

			By("checking if extra-cm-file volumes are mounted to the Backstage container")
			g.Expect("/my/file/path/cm11").To(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/cm12").To(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/cm21").To(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/cm22").NotTo(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/cm.31").To(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/cm.31").To(BeMountedToContainer(sidecarContainer))
			g.Expect("/cm/file/withpath").To(BeMountedToContainer(backstageContainer))

			By("checking if extra-secret-file volumes are added to PodSpec")
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret("secret-file1")).To(BeAddedAsVolumeToPodSpec(podSpec))
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret("secret-file2")).To(BeAddedAsVolumeToPodSpec(podSpec))
			g.Expect(utils.GenerateVolumeNameFromCmOrSecret("secret-file3.dot")).To(BeAddedAsVolumeToPodSpec(podSpec))

			By("checking if extra-secret-file volumes are mounted to the Backstage container")

			g.Expect("/my/file/path/sec11").To(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/sec12").NotTo(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/sec21").To(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/sec22").NotTo(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/sec.31").NotTo(BeMountedToContainer(backstageContainer))
			g.Expect("/my/file/path/sec.31").To(BeMountedToContainer(sidecarContainer))
			g.Expect("/secret/file/withpath").To(BeMountedToContainer(backstageContainer))

			By("checking if extra-envvars are injected to the Backstage container as EnvFrom")
			g.Expect("cm-env1").To(BeEnvFromForContainer(backstageContainer))

			By("checking if environment variables are injected to the Backstage container as EnvVar")
			g.Expect("env1").To(BeEnvVarForContainer(backstageContainer))
			g.Expect("cm21").To(BeEnvVarForContainer(sidecarContainer))
			g.Expect("cm21").To(BeEnvVarForContainer(backstageContainer))
			g.Expect("sec11").To(BeEnvVarForContainer(backstageContainer))

		}, time.Minute, time.Second).Should(Succeed(), controllerMessage())
	})

	It("generates label and annotation", func() {

		appConfig := generateConfigMap(ctx, k8sClient, "app-config1", ns, map[string]string{"key11": "app:", "key12": "app:"}, nil, nil)

		bs := bsv1.BackstageSpec{
			Application: &bsv1.Application{
				AppConfig: &bsv1.AppConfig{
					ConfigMaps: []bsv1.FileObjectRef{
						{Name: appConfig},
					},
				},
			},
		}

		backstageName := createAndReconcileBackstage(ctx, ns, bs, "")
		Eventually(func(g Gomega) {

			cm := &corev1.ConfigMap{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: appConfig}, cm)
			g.Expect(err).ShouldNot(HaveOccurred())

			g.Expect(cm.Labels).To(HaveLen(1))
			g.Expect(cm.Labels[model.ExtConfigSyncLabel]).To(Equal("true"))

			g.Expect(cm.Annotations).To(HaveLen(1))
			g.Expect(cm.Annotations[model.BackstageNameAnnotation]).To(Equal(backstageName))

		}, 10*time.Second, time.Second).Should(Succeed())

	})

	//It("creates default Backstage and then update this CR", func() {
	//
	//	backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{}, "")
	//
	//	Eventually(func(g Gomega) {
	//		By("creating Deployment with replicas=1 by default")
	//		deploy := &appsv1.Deployment{}
	//		err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
	//		g.Expect(err).To(Not(HaveOccurred()))
	//		g.Expect(deploy.Spec.Replicas).To(HaveValue(BeEquivalentTo(1)))
	//		g.Expect(deploy.Spec.Template.Spec.ImagePullSecrets).To(HaveLen(0))
	//
	//	}, time.Minute, time.Second).Should(Succeed())
	//
	//	By("updating Backstage")
	//	imageName := "quay.io/my-org/my-awesome-image:1.2.3"
	//	ips := []string{"some-image-pull-secret-1", "some-image-pull-secret-2"}
	//	update := &bsv1.Backstage{}
	//	err := k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, update)
	//	Expect(err).To(Not(HaveOccurred()))
	//	update.Spec.Application = &bsv1.Application{}
	//	update.Spec.Application.Replicas = ptr.To(int32(2))
	//	update.Spec.Application.Image = ptr.To(imageName)
	//	update.Spec.Application.ImagePullSecrets = ips
	//
	//	err = k8sClient.Update(ctx, update)
	//	Expect(err).To(Not(HaveOccurred()))
	//	_, err = NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
	//		NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
	//	})
	//	Expect(err).To(Not(HaveOccurred()))
	//
	//	Eventually(func(g Gomega) {
	//
	//		deploy := &appsv1.Deployment{}
	//		err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
	//		g.Expect(err).To(Not(HaveOccurred()))
	//		g.Expect(deploy.Spec.Replicas).To(HaveValue(BeEquivalentTo(2)))
	//		g.Expect(deploy.Spec.Template.Spec.ImagePullSecrets).To(HaveLen(2))
	//		g.Expect(deploy.Spec.Template.Spec.Containers[model.BackstageContainerIndex(deploy)].Image).To(HaveValue(BeEquivalentTo(imageName)))
	//
	//	}, time.Minute, time.Second).Should(Succeed())
	//
	//})

	It("creates Backstage deployment with spec.deployment ", func() {

		bs2 := &bsv1.Backstage{}

		err := ReadYamlFile("testdata/spec-deployment.yaml", bs2)
		Expect(err).To(Not(HaveOccurred()))

		backstageName := createAndReconcileBackstage(ctx, ns, bs2.Spec, "")

		Eventually(func(g Gomega) {
			By("creating Deployment ")
			deploy := &appsv1.Deployment{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, deploy)
			g.Expect(err).To(Not(HaveOccurred()))
			var bscontainer corev1.Container
			for _, c := range deploy.Spec.Template.Spec.Containers {

				if c.Name == "backstage-backend" {
					bscontainer = c
					break
				}
			}

			g.Expect(bscontainer).NotTo(BeNil())
			g.Expect(bscontainer.Image).To(HaveValue(Equal("busybox")))

			var bsvolume corev1.Volume
			for _, v := range deploy.Spec.Template.Spec.Volumes {

				if v.Name == "my-volume" {
					bsvolume = v
					break
				}
			}

			g.Expect(bsvolume).NotTo(BeNil())
			g.Expect(bsvolume.Ephemeral).NotTo(BeNil())
			g.Expect(*bsvolume.Ephemeral.VolumeClaimTemplate.Spec.StorageClassName).To(Equal("special"))

		}, 10*time.Second, time.Second).Should(Succeed())

	})

})

// Duplicated files in different CMs
// Message: "Deployment.apps \"test-backstage-ro86g-deployment\" is invalid: spec.template.spec.containers[0].volumeMounts[4].mountPath: Invalid value: \"/my/mount/path/key12\": must be unique",

// No CM configured
//failed to preprocess backstage spec app-configs failed to get configMap app-config3: configmaps "app-config3" not found

// If no such a key - no reaction, it is just not included

// mounting/injecting secret by key only
