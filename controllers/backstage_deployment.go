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

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	bs "janus-idp.io/backstage-operator/api/v1alpha1"
)

const (
	_defaultBackstageInitContainerName = "install-dynamic-plugins"
	_defaultBackstageMainContainerName = "backstage-backend"
	_containersWorkingDir              = "/opt/app-root/src"
)

//var (
//	DefaultBackstageDeployment = fmt.Sprintf(`
//apiVersion: apps/v1
//kind: Deployment
//metadata:
// name: backstage
//spec:
// replicas: 1
// selector:
//   matchLabels:
//     janus-idp.io/app:  # placeholder for 'backstage-<cr-name>'
// template:
//   metadata:
//     labels:
//       janus-idp.io/app:  # placeholder for 'backstage-<cr-name>'
//   spec:
//#      serviceAccountName: default
//
//     volumes:
//       - ephemeral:
//           volumeClaimTemplate:
//             spec:
//               accessModes:
//               - ReadWriteOnce
//               resources:
//                 requests:
//                   storage: 1Gi
//         name: dynamic-plugins-root
//       - name: dynamic-plugins-npmrc
//         secret:
//           defaultMode: 420
//           optional: true
//           secretName: dynamic-plugins-npmrc
//
//     initContainers:
//       - command:
//         - ./install-dynamic-plugins.sh
//         - /dynamic-plugins-root
//         env:
//         - name: NPM_CONFIG_USERCONFIG
//           value: %[3]s/.npmrc.dynamic-plugins
//         image: 'quay.io/janus-idp/backstage-showcase:next'
//         imagePullPolicy: IfNotPresent
//         name: %[1]s
//         volumeMounts:
//         - mountPath: /dynamic-plugins-root
//           name: dynamic-plugins-root
//         - mountPath: %[3]s/.npmrc.dynamic-plugins
//           name: dynamic-plugins-npmrc
//           readOnly: true
//           subPath: .npmrc
//         workingDir: %[3]s
//
//     containers:
//       - name: %[2]s
//         image: quay.io/janus-idp/backstage-showcase:next
//         imagePullPolicy: IfNotPresent
//         args:
//           - "--config"
//           - "dynamic-plugins-root/app-config.dynamic-plugins.yaml"
//         readinessProbe:
//           failureThreshold: 3
//           httpGet:
//             path: /healthcheck
//             port: 7007
//             scheme: HTTP
//           initialDelaySeconds: 30
//           periodSeconds: 10
//           successThreshold: 2
//           timeoutSeconds: 2
//         livenessProbe:
//           failureThreshold: 3
//           httpGet:
//             path: /healthcheck
//             port: 7007
//             scheme: HTTP
//           initialDelaySeconds: 60
//           periodSeconds: 10
//           successThreshold: 1
//           timeoutSeconds: 2
//         ports:
//           - name: http
//             containerPort: 7007
//         env:
//           - name: APP_CONFIG_backend_listen_port
//             value: "7007"
//         envFrom:
//           - secretRef:
//               name: postgres-secrets
//#            - secretRef:
//#                name: backstage-secrets
//         volumeMounts:
//           - mountPath: %[3]s/dynamic-plugins-root
//             name: dynamic-plugins-root
//`, _defaultBackstageInitContainerName, _defaultBackstageMainContainerName, _containersWorkingDir)
//)

// ContainerVisitor is called with each container
type ContainerVisitor func(container *v1.Container)

// visitContainers invokes the visitor function for every container in the given pod template spec
func visitContainers(podTemplateSpec *v1.PodTemplateSpec, visitor ContainerVisitor) {
	for i := range podTemplateSpec.Spec.InitContainers {
		visitor(&podTemplateSpec.Spec.InitContainers[i])
	}
	for i := range podTemplateSpec.Spec.Containers {
		visitor(&podTemplateSpec.Spec.Containers[i])
	}
	for i := range podTemplateSpec.Spec.EphemeralContainers {
		visitor((*v1.Container)(&podTemplateSpec.Spec.EphemeralContainers[i].EphemeralContainerCommon))
	}
}

func (r *BackstageReconciler) reconcileBackstageDeployment(ctx context.Context, backstage bs.Backstage, ns string) error {
	deployment := &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{
		Name:      getDefaultObjName(backstage),
		Namespace: ns},
	}
	if _, err := controllerutil.CreateOrUpdate(ctx, r.Client, deployment, r.deploymentObjectMutFun(ctx, deployment, backstage, ns)); err != nil {
		if errors.IsConflict(err) {
			return fmt.Errorf("retry sync needed: %v", err)
		}
		return err
	}
	return nil
}

func (r *BackstageReconciler) deploymentObjectMutFun(ctx context.Context, targetDeployment *appsv1.Deployment, backstage bs.Backstage, ns string) controllerutil.MutateFn {
	return func() error {
		deployment := &appsv1.Deployment{}
		targetDeployment.ObjectMeta.DeepCopyInto(&deployment.ObjectMeta)

		err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "deployment.yaml", ns, deployment)
		if err != nil {
			return fmt.Errorf("failed to read config: %s", err)
		}

		// Override deployment name
		deployment.Name = getDefaultObjName(backstage)

		r.setDefaultDeploymentImage(deployment)

		r.applyBackstageLabels(backstage, deployment)

		if err = r.addParams(ctx, backstage, ns, deployment); err != nil {
			return err
		}

		r.applyApplicationParamsFromCR(backstage, deployment)

		if err = r.validateAndUpdatePsqlSecretRef(backstage, deployment); err != nil {
			return fmt.Errorf("failed to validate database secret, reason: %s", err)
		}

		if r.OwnsRuntime {
			if err = controllerutil.SetControllerReference(&backstage, deployment, r.Scheme); err != nil {
				return fmt.Errorf("failed to set owner reference: %s", err)
			}
		}

		deployment.ObjectMeta.DeepCopyInto(&targetDeployment.ObjectMeta)
		deployment.Spec.DeepCopyInto(&targetDeployment.Spec)
		return nil
	}
}

func (r *BackstageReconciler) addParams(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	if err := r.addVolumes(ctx, backstage, ns, deployment); err != nil {
		return fmt.Errorf("failed to add volumes to Backstage deployment, reason: %s", err)
	}

	if err := r.addVolumeMounts(ctx, backstage, ns, deployment); err != nil {
		return fmt.Errorf("failed to add volume mounts to Backstage deployment, reason: %s", err)
	}

	if err := r.addContainerArgs(ctx, backstage, ns, deployment); err != nil {
		return fmt.Errorf("failed to add container args to Backstage deployment, reason: %s", err)
	}

	if err := r.addEnvVars(backstage, ns, deployment); err != nil {
		return fmt.Errorf("failed to add env vars to Backstage deployment, reason: %s", err)
	}
	return nil
}

func (r *BackstageReconciler) addVolumes(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	dpConfVol, err := r.getDynamicPluginsConfVolume(ctx, backstage, ns)
	if err != nil {
		return err
	}
	if dpConfVol != nil {
		deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, *dpConfVol)
	}

	backendAuthAppConfig, err := r.getBackendAuthAppConfig(ctx, backstage, ns)
	if err != nil {
		return err
	}

	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, r.appConfigsToVolumes(backstage, backendAuthAppConfig)...)
	deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes, r.extraFilesToVolumes(backstage)...)
	return nil
}

func (r *BackstageReconciler) addVolumeMounts(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	err := r.addDynamicPluginsConfVolumeMount(ctx, backstage, ns, deployment)
	if err != nil {
		return err
	}
	backendAuthAppConfig, err := r.getBackendAuthAppConfig(ctx, backstage, ns)
	if err != nil {
		return err
	}
	err = r.addAppConfigsVolumeMounts(ctx, backstage, ns, deployment, backendAuthAppConfig)
	if err != nil {
		return err
	}
	return r.addExtraFilesVolumeMounts(ctx, backstage, ns, deployment)
}

func (r *BackstageReconciler) addContainerArgs(ctx context.Context, backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	backendAuthAppConfig, err := r.getBackendAuthAppConfig(ctx, backstage, ns)
	if err != nil {
		return err
	}
	return r.addAppConfigsContainerArgs(ctx, backstage, ns, deployment, backendAuthAppConfig)
}

func (r *BackstageReconciler) addEnvVars(backstage bs.Backstage, ns string, deployment *appsv1.Deployment) error {
	r.addExtraEnvs(backstage, deployment)
	return nil
}

func (r *BackstageReconciler) validateAndUpdatePsqlSecretRef(backstage bs.Backstage, deployment *appsv1.Deployment) error {
	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name != _defaultBackstageMainContainerName {
			continue
		}
		for k, from := range deployment.Spec.Template.Spec.Containers[i].EnvFrom {
			if from.SecretRef.Name != postGresSecret {
				continue
			}
			if len(backstage.Spec.Database.AuthSecretName) == 0 {
				from.SecretRef.Name = getDefaultPsqlSecretName(&backstage)
			} else {
				from.SecretRef.Name = backstage.Spec.Database.AuthSecretName
			}
			deployment.Spec.Template.Spec.Containers[i].EnvFrom[k] = from
			break
		}
	}

	return nil
}

func (r *BackstageReconciler) setDefaultDeploymentImage(deployment *appsv1.Deployment) {
	visitContainers(&deployment.Spec.Template, func(container *v1.Container) {
		if len(container.Image) == 0 || container.Image == fmt.Sprintf("<%s>", bs.EnvBackstageImage) {
			container.Image = r.BackstageImage
		}
	})
}

func (r *BackstageReconciler) applyBackstageLabels(backstage bs.Backstage, deployment *appsv1.Deployment) {
	setBackstageAppLabel(&deployment.Spec.Template.ObjectMeta.Labels, backstage)
	setBackstageAppLabel(&deployment.Spec.Selector.MatchLabels, backstage)
	r.labels(&deployment.ObjectMeta, backstage)
}

func (r *BackstageReconciler) applyApplicationParamsFromCR(backstage bs.Backstage, deployment *appsv1.Deployment) {
	if backstage.Spec.Application != nil {
		deployment.Spec.Replicas = backstage.Spec.Application.Replicas
		if backstage.Spec.Application.Image != nil {
			visitContainers(&deployment.Spec.Template, func(container *v1.Container) {
				container.Image = *backstage.Spec.Application.Image
			})
		}
		if backstage.Spec.Application.ImagePullSecrets != nil { // use image pull secrets from the CR spec
			deployment.Spec.Template.Spec.ImagePullSecrets = nil
			if len(*backstage.Spec.Application.ImagePullSecrets) > 0 {
				for _, imagePullSecret := range *backstage.Spec.Application.ImagePullSecrets {
					deployment.Spec.Template.Spec.ImagePullSecrets = append(deployment.Spec.Template.Spec.ImagePullSecrets, v1.LocalObjectReference{
						Name: imagePullSecret,
					})
				}
			}
		}
	}
}

func getDefaultObjName(backstage bs.Backstage) string {
	return fmt.Sprintf("backstage-%s", backstage.Name)
}

func getDefaultDbObjName(backstage bs.Backstage) string {
	return fmt.Sprintf("backstage-psql-%s", backstage.Name)
}
