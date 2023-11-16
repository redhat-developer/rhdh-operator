/*
Copyright 2023.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"time"

	bsv1alphav1 "backstage.io/backstage-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var _ = Describe("Backstage controller", func() {
	var (
		ctx               context.Context
		backstageName     string
		namespace         *corev1.Namespace
		typeNamespaceName types.NamespacedName
	)

	BeforeEach(func() {
		ctx = context.Background()
		ns := fmt.Sprintf("ns-%d-%s", GinkgoParallelProcess(), randString(5))
		backstageName = "test-backstage-" + randString(5)
		namespace = &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name:      ns,
				Namespace: ns,
			},
		}
		typeNamespaceName = types.NamespacedName{Name: backstageName, Namespace: ns}

		By("Creating the Namespace to perform the tests")
		err := k8sClient.Create(ctx, namespace)
		Expect(err).To(Not(HaveOccurred()))
	})

	AfterEach(func() {
		// TODO(user): Attention if you improve this code by adding other context test you MUST
		// be aware of the current delete namespace limitations. More info: https://book.kubebuilder.io/reference/envtest.html#testing-considerations
		By("Deleting the Namespace to perform the tests")
		_ = k8sClient.Delete(ctx, namespace)
	})

	When("creating default CR with no spec", func() {
		var backstage *bsv1alphav1.Backstage
		BeforeEach(func() {
			backstage = &bsv1alphav1.Backstage{
				ObjectMeta: metav1.ObjectMeta{
					Name:      backstageName,
					Namespace: namespace.Name,
				},
				Spec: bsv1alphav1.BackstageSpec{},
			}

			err := k8sClient.Create(ctx, backstage)
			Expect(err).To(Not(HaveOccurred()))
		})

		It("should successfully reconcile a custom resource for default Backstage", func() {
			By("Checking if the custom resource was successfully created")
			Eventually(func() error {
				found := &bsv1alphav1.Backstage{}
				return k8sClient.Get(ctx, typeNamespaceName, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Reconciling the custom resource created")
			backstageReconciler := &BackstageReconciler{
				Client:    k8sClient,
				Scheme:    k8sClient.Scheme(),
				Namespace: namespace.Name,
			}

			_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespaceName,
			})
			Expect(err).To(Not(HaveOccurred()))

			By("Checking if Deployment was successfully created in the reconciliation")
			Eventually(func() error {
				found := &appsv1.Deployment{}
				// TODO to get name from default
				return k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: "backstage"}, found)
			}, time.Minute, time.Second).Should(Succeed())

			By("Checking the latest Status added to the Backstage instance")
			Eventually(func() error {
				//TODO the status is under construction
				err = k8sClient.Get(ctx, typeNamespaceName, backstage)
				if err != nil {
					return err
				}
				if backstage.Status.BackstageState != "deployed" {
					return fmt.Errorf("The status is not 'deployed' '%s'", backstage.Status)
				}
				return nil
			}, time.Minute, time.Second).Should(Succeed())
		})
	})

	Context("specifying runtime configs", func() {
		When("creating CR with runtime config for Backstage deployment", func() {
			var backstage *bsv1alphav1.Backstage

			BeforeEach(func() {
				backstageConfigMap := &corev1.ConfigMap{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "ConfigMap",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-bs-config",
						Namespace: namespace.Name,
					},
					Data: map[string]string{
						"deploy": `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: bs1-deployment
  labels:
    app: bs1
spec:
  replicas: 1
  selector:
    matchLabels:
      app: bs1
  template:
    metadata:
      labels:
        app: bs1
    spec:
      containers:
        - name: bs1
          image: busybox
`,
					},
				}
				err := k8sClient.Create(ctx, backstageConfigMap)
				Expect(err).To(Not(HaveOccurred()))

				backstage = &bsv1alphav1.Backstage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backstageName,
						Namespace: namespace.Name,
					},
					Spec: bsv1alphav1.BackstageSpec{
						RuntimeConfig: bsv1alphav1.RuntimeConfig{
							BackstageConfigName: backstageConfigMap.Name,
						},
					},
				}

				err = k8sClient.Create(ctx, backstage)
				Expect(err).To(Not(HaveOccurred()))
			})

			It("should create the resources", func() {
				By("Checking if the custom resource was successfully created")
				Eventually(func() error {
					found := &bsv1alphav1.Backstage{}
					return k8sClient.Get(ctx, typeNamespaceName, found)
				}, time.Minute, time.Second).Should(Succeed())

				By("Reconciling the custom resource created")
				backstageReconciler := &BackstageReconciler{
					Client:    k8sClient,
					Scheme:    k8sClient.Scheme(),
					Namespace: namespace.Name,
				}
				_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespaceName,
				})
				Expect(err).To(Not(HaveOccurred()))

				By("Checking if Deployment was successfully created in the reconciliation")
				Eventually(func() error {
					found := &appsv1.Deployment{}
					// TODO to get name from default
					return k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: "bs1-deployment"}, found)
				}, time.Minute, time.Second).Should(Succeed())

				By("Checking the latest Status added to the Backstage instance")
				Eventually(func() error {
					//TODO the status is under construction
					err := k8sClient.Get(ctx, typeNamespaceName, backstage)
					if err != nil {
						return err
					}
					if backstage.Status.BackstageState != "deployed" {
						return fmt.Errorf("The status is not 'deployed' '%s'", backstage.Status)
					}
					return nil
				}, time.Minute, time.Second).Should(Succeed())
			})
		})

		When("creating CR with runtime config for the database", func() {
			var backstage *bsv1alphav1.Backstage

			BeforeEach(func() {
				localDbConfigMap := &corev1.ConfigMap{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "ConfigMap",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:      "my-db-config",
						Namespace: namespace.Name,
					},
					Data: map[string]string{
						"deployment": `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: db-deployment
spec:
  replicas: 1
  selector:
    matchLabels:
      app: db
  template:
    metadata:
      labels:
        app: db
    spec:
      containers:
        - name: db
          image: busybox
`,
					},
				}
				err := k8sClient.Create(ctx, localDbConfigMap)
				Expect(err).To(Not(HaveOccurred()))

				backstage = &bsv1alphav1.Backstage{
					ObjectMeta: metav1.ObjectMeta{
						Name:      backstageName,
						Namespace: namespace.Name,
					},
					Spec: bsv1alphav1.BackstageSpec{
						RuntimeConfig: bsv1alphav1.RuntimeConfig{
							LocalDbConfigName: localDbConfigMap.Name,
						},
					},
				}

				err = k8sClient.Create(ctx, backstage)
				Expect(err).To(Not(HaveOccurred()))
			})

			It("should create the resources", func() {
				By("Checking if the custom resource was successfully created")
				Eventually(func() error {
					found := &bsv1alphav1.Backstage{}
					return k8sClient.Get(ctx, typeNamespaceName, found)
				}, time.Minute, time.Second).Should(Succeed())

				By("Reconciling the custom resource created")
				backstageReconciler := &BackstageReconciler{
					Client:    k8sClient,
					Scheme:    k8sClient.Scheme(),
					Namespace: namespace.Name,
				}
				_, err := backstageReconciler.Reconcile(ctx, reconcile.Request{
					NamespacedName: typeNamespaceName,
				})
				Expect(err).To(Not(HaveOccurred()))

				By("Checking if StatefulSet was successfully created in the reconciliation")
				Eventually(func() error {
					found := &appsv1.Deployment{}
					// TODO to get name from default
					return k8sClient.Get(ctx, types.NamespacedName{Namespace: namespace.Name, Name: "db-deployment"}, found)
				}, time.Minute, time.Second).Should(Succeed())

				By("Checking the latest Status added to the Backstage instance")
				Eventually(func() error {
					//TODO the status is under construction
					err := k8sClient.Get(ctx, typeNamespaceName, backstage)
					if err != nil {
						return err
					}
					if backstage.Status.BackstageState != "deployed" {
						return fmt.Errorf("The status is not 'deployed' '%s'", backstage.Status)
					}
					return nil
				}, time.Minute, time.Second).Should(Succeed())
			})
		})
	})
})
