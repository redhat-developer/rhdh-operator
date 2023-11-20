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

	bs "backstage.io/backstage-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	_defaultBackstageMainContainerName = "backstage-backend"
)

var (
	DefaultBackstageDeployment = fmt.Sprintf(`
apiVersion: apps/v1
kind: Deployment
metadata:
  name: backstage
spec:
  replicas: 1
  selector:
    matchLabels:
      backstage.io/app:  # placeholder for 'backstage-<cr-name>'
  template:
    metadata:
      labels:
        backstage.io/app:  # placeholder for 'backstage-<cr-name>'
    spec:
#      serviceAccountName: default

      volumes:
        - ephemeral:
            volumeClaimTemplate:
              spec:
                accessModes:
                - ReadWriteOnce
                resources:
                  requests:
                    storage: 1Gi
          name: dynamic-plugins-root
        - configMap:
            defaultMode: 420
            name: dynamic-plugins
            optional: true
          name: dynamic-plugins
        - name: dynamic-plugins-npmrc
          secret:
            defaultMode: 420
            optional: true
            secretName: dynamic-plugins-npmrc
# TODO(rm3l): to mount if value set in CR
        #- name: backstage-app-config
        #  configMap:
        #    name: my-backstage-from-helm-app-config

      initContainers:
        - command:
          - ./install-dynamic-plugins.sh
          - /dynamic-plugins-root
          env:
          - name: NPM_CONFIG_USERCONFIG
            value: /opt/app-root/src/.npmrc.dynamic-plugins
          image: 'quay.io/janus-idp/backstage-showcase:next'
          imagePullPolicy: IfNotPresent
          name: install-dynamic-plugins
          volumeMounts:
          - mountPath: /dynamic-plugins-root
            name: dynamic-plugins-root
          - mountPath: /opt/app-root/src/dynamic-plugins.yaml
            name: dynamic-plugins
            readOnly: true
            subPath: dynamic-plugins.yaml
          - mountPath: /opt/app-root/src/.npmrc.dynamic-plugins
            name: dynamic-plugins-npmrc
            readOnly: true
            subPath: .npmrc
          workingDir: /opt/app-root/src

      containers:
        - name: %s
          image: quay.io/janus-idp/backstage-showcase:next
          imagePullPolicy: IfNotPresent
          args:
            - "--config"
            - "dynamic-plugins-root/app-config.dynamic-plugins.yaml"
# TODO(rm3l): to mount if value set in CR
            #- "--config"
            #- "/opt/app-root/src/app-config-from-configmap.yaml"
          readinessProbe:
            failureThreshold: 3
            httpGet:
              path: /healthcheck
              port: 7007
              scheme: HTTP
            initialDelaySeconds: 30
            periodSeconds: 10
            successThreshold: 2
            timeoutSeconds: 2
          livenessProbe:
            failureThreshold: 3
            httpGet:
              path: /healthcheck
              port: 7007
              scheme: HTTP
            initialDelaySeconds: 60
            periodSeconds: 10
            successThreshold: 1
            timeoutSeconds: 2
          ports:
            - name: http
              containerPort: 7007
# TODO Handle user-defined env vars
          env:
            - name: APP_CONFIG_backend_listen_port
              value: "7007"
          envFrom:
            - secretRef:
                name: postgres-secrets
#            - secretRef:
#                name: backstage-secrets
          volumeMounts:
# TODO(rm3l): to mount if value set in CR
            #- name: backstage-app-config
            #  mountPath: "/opt/app-root/src/app-config-from-configmap.yaml"
            #  subPath: app-config.yaml
            - mountPath: /opt/app-root/src/dynamic-plugins-root
              name: dynamic-plugins-root
`, _defaultBackstageMainContainerName)
)

func (r *BackstageReconciler) applyBackstageDeployment(ctx context.Context, backstage bs.Backstage, ns string) error {

	//lg := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	err := r.readConfigMapOrDefault(ctx, backstage.Spec.RawRuntimeConfig.BackstageConfigName, "deploy", ns, DefaultBackstageDeployment, deployment)
	if err != nil {
		return fmt.Errorf("failed to read config: %s", err)
	}

	foundDeployment := &appsv1.Deployment{}
	err = r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: ns}, foundDeployment)
	if err != nil {
		if errors.IsNotFound(err) {

			setBackstageAppLabel(&deployment.Spec.Template.ObjectMeta.Labels, backstage)
			setBackstageAppLabel(&deployment.Spec.Selector.MatchLabels, backstage)
			r.labels(&deployment.ObjectMeta, backstage)

			if r.OwnsRuntime {
				if err := controllerutil.SetControllerReference(&backstage, deployment, r.Scheme); err != nil {
					return fmt.Errorf("failed to set owner reference: %s", err)
				}
			}

			r.addVolumes(backstage, deployment)

			if backstage.Spec.RawRuntimeConfig.BackstageConfigName == "" {
				var appConfigFileNamesMap map[string][]string
				appConfigFileNamesMap, err = r.extractAppConfigFileNames(ctx, backstage, ns)
				if err != nil {
					return err
				}
				r.addVolumeMounts(deployment, appConfigFileNamesMap)
				r.addContainerArgs(deployment, appConfigFileNamesMap)
				r.addEnvVars(deployment)
			}

			err = r.Create(ctx, deployment)
			if err != nil {
				return fmt.Errorf("failed to create backstage deplyment, reason: %s", err)
			}

		} else {
			return fmt.Errorf("failed to get backstage deployment, reason: %s", err)
		}
	} else {
		//lg.Info("CR update is ignored for the time")
		err = r.Update(ctx, foundDeployment)
		if err != nil {
			return fmt.Errorf("failed to update backstage deplyment, reason: %s", err)
		}
	}
	return nil
}

func (r *BackstageReconciler) addVolumes(backstage bs.Backstage, deployment *appsv1.Deployment) {
	for _, appConfig := range backstage.Spec.AppConfigs {
		var volumeSource v1.VolumeSource
		switch appConfig.Kind {
		case "ConfigMap":
			volumeSource.ConfigMap = &v1.ConfigMapVolumeSource{
				DefaultMode:          pointer.Int32(420),
				LocalObjectReference: v1.LocalObjectReference{Name: appConfig.Name},
			}
		case "Secret":
			volumeSource.Secret = &v1.SecretVolumeSource{
				DefaultMode: pointer.Int32(420),
				SecretName:  appConfig.Name,
			}
		}
		deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes,
			v1.Volume{
				Name:         appConfig.Name,
				VolumeSource: volumeSource,
			})
	}
}

func (r *BackstageReconciler) addVolumeMounts(deployment *appsv1.Deployment, appConfigFileNamesMap map[string][]string) {
	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			for appConfigName := range appConfigFileNamesMap {
				deployment.Spec.Template.Spec.Containers[i].VolumeMounts = append(deployment.Spec.Template.Spec.Containers[i].VolumeMounts,
					v1.VolumeMount{
						Name:      appConfigName,
						MountPath: "/opt/app-root/src/" + appConfigName,
					})
			}
			break
		}
	}
}

func (r *BackstageReconciler) addContainerArgs(deployment *appsv1.Deployment, appConfigFileNamesMap map[string][]string) {
	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			for appConfigName, fileNames := range appConfigFileNamesMap {
				// Args
				for _, fileName := range fileNames {
					deployment.Spec.Template.Spec.Containers[i].Args =
						append(deployment.Spec.Template.Spec.Containers[i].Args, "--config",
							fmt.Sprintf("/opt/app-root/src/%s/%s", appConfigName, fileName))
				}
			}
			break
		}
	}
}

func (r *BackstageReconciler) addEnvVars(deployment *appsv1.Deployment) {
	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			// FIXME(rm3l): Hack to set the 'BACKEND_SECRET' env var
			deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env, v1.EnvVar{
				Name:  "BACKEND_SECRET",
				Value: "ch4ng3M3",
			})
			break
		}
	}
}

func (r *BackstageReconciler) extractAppConfigFileNames(ctx context.Context, backstage bs.Backstage, ns string) (map[string][]string, error) {
	m := make(map[string][]string)
	for _, appConfig := range backstage.Spec.AppConfigs {
		switch appConfig.Kind {
		case "ConfigMap":
			cm := v1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: appConfig.Name, Namespace: ns}, &cm); err != nil {
				return nil, err
			}
			for filename := range cm.Data {
				m[appConfig.Name] = append(m[appConfig.Name], filename)
			}
		case "Secret":
			sec := v1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Name: appConfig.Name, Namespace: ns}, &sec); err != nil {
				return nil, err
			}
			for filename := range sec.Data {
				m[appConfig.Name] = append(m[appConfig.Name], filename)
			}
		}
	}
	return m, nil
}
