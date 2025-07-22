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
	"time"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"

	storagev1 "k8s.io/api/storage/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/utils/ptr"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	corev1 "k8s.io/api/core/v1"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

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
			g.Expect(path).To(BeMountedToContainer(depl.Spec.Template.Spec.Containers[model.BackstageContainerIndex(depl)]))
			g.Expect(utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim1"))).
				To(BeAddedAsVolumeToPodSpec(depl.Spec.Template.Spec))

			path = filepath.Join(model.DefaultMountDir, utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim2")))
			g.Expect(path).To(BeMountedToContainer(depl.Spec.Template.Spec.Containers[model.BackstageContainerIndex(depl)]))
			g.Expect(utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim2"))).
				To(BeAddedAsVolumeToPodSpec(depl.Spec.Template.Spec))

			pod, err := getBackstagePod(ctx, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())

			// check if mounted directory is there
			_, _, err = executeRemoteCommand(ctx, ns, pod.Name, backstageContainerName(depl), fmt.Sprintf("test -d %s", path))
			g.Expect(err).ShouldNot(HaveOccurred())

		}, 5*time.Minute, time.Second).Should(Succeed(), controllerMessage())
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
				PersistentVolumeReclaimPolicy: corev1.PersistentVolumeReclaimDelete,
				StorageClassName:              scName,
				PersistentVolumeSource: corev1.PersistentVolumeSource{
					HostPath: &corev1.HostPathVolumeSource{
						Path: "/tmp",
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
			g.Expect(path).To(BeMountedToContainer(depl.Spec.Template.Spec.Containers[model.BackstageContainerIndex(depl)]))
			g.Expect(utils.ToRFC1123Label(model.PvcsName(backstageName, "myclaim1"))).
				To(BeAddedAsVolumeToPodSpec(depl.Spec.Template.Spec))

			pod, err := getBackstagePod(ctx, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())
			//g.Expect(pod.Spec.Containers[model.BackstageContainerIndex(depl)].Name).To(Equal("backstage-backend"))

			// check if mounted directory is there
			_, _, err = executeRemoteCommand(ctx, ns, pod.Name, backstageContainerName(depl), fmt.Sprintf("test -d %s", path))
			g.Expect(err).ShouldNot(HaveOccurred())

		}, 5*time.Minute, time.Second).Should(Succeed(), controllerMessage())

	})

	It("creates specified PVCs and mounts to container", func() {

		if !*testEnv.UseExistingCluster {
			Skip("Skipped for not real cluster")
		}

		pvc1 := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-pvc1",
				Namespace: ns,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}
		err := k8sClient.Create(ctx, &pvc1)
		Expect(err).ShouldNot(HaveOccurred())

		pvc2 := corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-pvc2",
				Namespace: ns,
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{
					corev1.ReadWriteOnce,
				},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse("1Gi"),
					},
				},
			},
		}
		err = k8sClient.Create(ctx, &pvc2)
		Expect(err).ShouldNot(HaveOccurred())

		pvcCm := generateConfigMap(ctx, k8sClient, "pvc-conf", ns,
			map[string]string{"pvcs.yaml": readTestYamlFile("raw-pvcs.yaml")}, nil, nil)

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1.BackstageSpec{
			Application: &bsv1.Application{
				ExtraFiles: &bsv1.ExtraFiles{
					Pvcs: []bsv1.PvcRef{
						{
							Name: "my-pvc1",
						},
						{
							Name:      "my-pvc2",
							MountPath: "/my/pvc2/path",
						},
					},
				},
			},
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
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "my-pvc1"}, pvc)
			g.Expect(err).ShouldNot(HaveOccurred())
			pvc2 := &corev1.PersistentVolumeClaim{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: "my-pvc2"}, pvc2)
			g.Expect(err).ShouldNot(HaveOccurred())

			// check if added to deployment
			depl := &appsv1.Deployment{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.DeploymentName(backstageName)}, depl)
			g.Expect(err).ShouldNot(HaveOccurred())
			path := filepath.Join(model.DefaultMountDir, "my-pvc1")
			g.Expect(path).To(BeMountedToContainer(depl.Spec.Template.Spec.Containers[model.BackstageContainerIndex(depl)]))
			g.Expect("my-pvc1").To(BeAddedAsVolumeToPodSpec(depl.Spec.Template.Spec))

			path2 := "/my/pvc2/path"
			g.Expect(path2).To(BeMountedToContainer(depl.Spec.Template.Spec.Containers[model.BackstageContainerIndex(depl)]))
			g.Expect("my-pvc2").To(BeAddedAsVolumeToPodSpec(depl.Spec.Template.Spec))

			pod, err := getBackstagePod(ctx, ns, backstageName)
			g.Expect(err).ShouldNot(HaveOccurred())

			// check if mounted directory is there
			_, _, err = executeRemoteCommand(ctx, ns, pod.Name, backstageContainerName(depl), fmt.Sprintf("test -d %s", path))
			g.Expect(err).ShouldNot(HaveOccurred())
			_, _, err = executeRemoteCommand(ctx, ns, pod.Name, backstageContainerName(depl), fmt.Sprintf("test -d %s", path2))
			g.Expect(err).ShouldNot(HaveOccurred())

		}, 5*time.Minute, time.Second).Should(Succeed(), controllerMessage())
	})

})
