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

package controller

/*
import (
	"context"
	"fmt"

	bs "janus-idp.io/backstage-operator/api/v1alpha1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

func (r *BackstageReconciler) applyPV(ctx context.Context, backstage bs.Backstage, ns string) error {
	// Postgre PersistentVolume
	//lg := log.FromContext(ctx)

	pv := &corev1.PersistentVolume{}
	err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.LocalDbConfigName, "db-pv.yaml", ns, pv)
	if err != nil {
		return err
	}

	err = r.Get(ctx, types.NamespacedName{Name: pv.Name, Namespace: ns}, pv)

	if err != nil {
		if errors.IsNotFound(err) {
		} else {
			return fmt.Errorf("failed to get PV, reason: %s", err)
		}
	} else {
		//lg.Info("CR update is ignored for the time")
		return nil
	}

	r.labels(&pv.ObjectMeta, backstage)
	if r.OwnsRuntime {
		if err := controllerutil.SetControllerReference(&backstage, pv, r.Scheme); err != nil {
			return fmt.Errorf("failed to set owner reference: %s", err)
		}
	}

	err = r.Create(ctx, pv)
	if err != nil {
		return fmt.Errorf("failed to create postgre persistent volume, reason:%s", err)
	}

	return nil
}
*/
