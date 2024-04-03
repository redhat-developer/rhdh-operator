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
	"redhat-developer/red-hat-developer-hub-operator/pkg/model"
	"time"

	openshift "github.com/openshift/api/route/v1"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	bsv1alpha1 "redhat-developer/red-hat-developer-hub-operator/api/v1alpha1"

	"k8s.io/apimachinery/pkg/types"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = When("create default backstage", func() {

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

	It("creates Backstage object (on Openshift)", func() {

		if !isOpenshiftCluster() {
			Skip("Skipped for non-Openshift cluster")
		}

		backstageName := createAndReconcileBackstage(ctx, ns, bsv1alpha1.BackstageSpec{
			Application: &bsv1alpha1.Application{
				Route: &bsv1alpha1.Route{
					//Host:      "localhost",
					//Enabled:   ptr.To(true),
					Subdomain: "test",
				},
			},
		})

		Eventually(func() error {
			found := &bsv1alpha1.Backstage{}
			return k8sClient.Get(ctx, types.NamespacedName{Name: backstageName, Namespace: ns}, found)
		}, time.Minute, time.Second).Should(Succeed())

		_, err := NewTestBackstageReconciler(ns).ReconcileAny(ctx, reconcile.Request{
			NamespacedName: types.NamespacedName{Name: backstageName, Namespace: ns},
		})
		Expect(err).To(Not(HaveOccurred()))

		Eventually(func(g Gomega) {
			By("creating Route")
			route := &openshift.Route{}
			err = k8sClient.Get(ctx, types.NamespacedName{Namespace: ns, Name: model.RouteName(backstageName)}, route)
			g.Expect(err).To(Not(HaveOccurred()), controllerMessage())

			g.Expect(route.Status.Ingress).To(HaveLen(1))
			g.Expect(route.Status.Ingress[0].Host).To(Not(BeEmpty()))

		}, 5*time.Minute, time.Second).Should(Succeed())

	})
})
