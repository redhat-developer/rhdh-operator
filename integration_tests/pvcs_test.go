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

package integration_tests

import (
	"context"
	"fmt"
	"path/filepath"
	"redhat-developer/red-hat-developer-hub-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"

	storagev1 "k8s.io/api/storage/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"time"

	corev1 "k8s.io/api/core/v1"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	bsv1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("create backstage PVCs configured", func() {

	var (
		ctx    context.Context
		ns     string
		scName string
		pvName string
	)

	BeforeEach(func() {
		ctx = context.Background()
		ns = createNamespace(ctx)
		scName = "my-pvctest-storage"
		pvName = fmt.Sprintf("testpv-%d-%s", GinkgoParallelProcess(), randString(5))
	})

	AfterEach(func() {
		deleteNamespace(ctx, ns)
		// Clean cluster scope objects
		_ = k8sClient.Delete(ctx, &corev1.PersistentVolume{ObjectMeta: metav1.ObjectMeta{Name: pvName}})
		_ = k8sClient.Delete(ctx, &storagev1.StorageClass{ObjectMeta: metav1.ObjectMeta{Name: scName}})
	})

	It("creates PV dynamically with configured by default PVC", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		pvcCm := generateConfigMap(ctx, k8sClient, "pvc-conf", ns,
			map[string]string{"pvcs.yaml": readTestYamlFile("raw-pvcs.yaml")}, nil, nil)

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
			RawRuntimeConfig: &bsv1.RuntimeConfig{
				BackstageConfigName: pvcCm,
			},
		}, "")

		Eventually(func(g Gomega) {

			pvc := &corev1.PersistentVolumeClaim{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.PvcsName(backstageName, "myclaim1")}, pvc)
			g.Expect(err).ShouldNot(HaveOccurred())
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.PvcsName(backstageName, "myclaim2")}, pvc)
			g.Expect(err).ShouldNot(HaveOccurred())

			// check if PVC is correctly created and initialized
			createdPvName := pvc.Spec.VolumeName
			g.Expect(createdPvName).NotTo(BeEmpty())
			g.Expect(*pvc.Spec.StorageClassName).NotTo(BeEmpty())

			// check if PV correctly created
			pv := &corev1.PersistentVolume{}
			err = k8sClient.Get(ctx, types.NamespacedName{Name: createdPvName}, pv)
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(pv.Spec.ClaimRef.Name).To(Equal(model.PvcsName(backstageName, "myclaim2")))
			g.Expect(pv.Status.Phase).To(Equal(corev1.VolumeBound))

			// check if added to deployment
			depl := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, depl)
			g.Expect(err).ShouldNot(HaveOccurred())

			path := filepath.Join(model.DefaultMountDir, utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim1")))
			g.Expect(path).To(BeMountedToContainer(depl.Spec.Template.Spec.Containers[0]))
			g.Expect(utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim1"))).
				To(BeAddedAsVolumeToPodSpec(depl.Spec.Template.Spec))

			path = filepath.Join(model.DefaultMountDir, utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim2")))
			g.Expect(path).To(BeMountedToContainer(depl.Spec.Template.Spec.Containers[0]))
			g.Expect(utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim2"))).
				To(BeAddedAsVolumeToPodSpec(depl.Spec.Template.Spec))

			podName, err := getBackstagePodName(ctx, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())
			// check if mounted directory is there
			_, _, err = executeRemoteCommand(ctx, ns, podName, "backstage-backend", fmt.Sprintf("test -d %s", path))
			g.Expect(err).ShouldNot(HaveOccurred())

		}, time.Minute, time.Second).Should(Succeed(), controllerMessage())
	})

	It("bounds configured by default PVC with precreated PV", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		//Precreate StorageClass
		sc := storagev1.StorageClass{
			ObjectMeta: metav1.ObjectMeta{
				Name: scName,
			},
			Provisioner: "kubernetes.io/no-provisioner",
		}
		err := k8sClient.Create(ctx, &sc)
		Expect(err).ShouldNot(HaveOccurred())

		// Precreate PV
		pv := corev1.PersistentVolume{
			ObjectMeta: metav1.ObjectMeta{
				Name: pvName,
			},
			Spec: corev1.PersistentVolumeSpec{
				Capacity: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("1Gi"),
				},
				VolumeMode: ptr.To(corev1.PersistentVolumeFilesystem),
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				StorageClassName: scName,
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/tmp/my/path",
					},
				},
			},
		}
		err = k8sClient.Create(ctx, &pv)
		Expect(err).ShouldNot(HaveOccurred())

		//Add PVC to Backstage CR configuration
		pvcCm := generateConfigMap(ctx, k8sClient, "pvc-conf", ns,
			map[string]string{"pvcs.yaml": readTestYamlFile("raw-pvcs2.yaml")}, nil, nil)

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
			RawRuntimeConfig: &bsv1.RuntimeConfig{
				BackstageConfigName: pvcCm,
			},
		}, "")

		Eventually(func(g Gomega) {

			pvc := &corev1.PersistentVolumeClaim{}
			err := k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.PvcsName(backstageName, "myclaim1")}, pvc)
			g.Expect(err).ShouldNot(HaveOccurred())

			// check if PVC is bound to precreated PV
			g.Expect(pvc.Spec.VolumeName).NotTo(BeEmpty())
			g.Expect(pvc.Spec.VolumeName).To(Equal(pvName))
			g.Expect(*pvc.Spec.StorageClassName).NotTo(BeEmpty())
			g.Expect(*pvc.Spec.StorageClassName).To(Equal(scName))

			// check if added to deployment
			depl := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, depl)
			g.Expect(err).ShouldNot(HaveOccurred())
			path := filepath.Join(model.DefaultMountDir, utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim1")))
			g.Expect(path).To(BeMountedToContainer(depl.Spec.Template.Spec.Containers[0]))
			g.Expect(utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim1"))).
				To(BeAddedAsVolumeToPodSpec(depl.Spec.Template.Spec))

			podName, err := getBackstagePodName(ctx, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())

			// check if mounted directory is there
			_, _, err = executeRemoteCommand(ctx, ns, podName, "backstage-backend", fmt.Sprintf("test -d %s", path))
			g.Expect(err).ShouldNot(HaveOccurred())

		}, time.Minute, time.Second).Should(Succeed(), controllerMessage())

	})

})
