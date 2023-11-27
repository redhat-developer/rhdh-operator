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
	"crypto/rand"
	"encoding/base64"
	"fmt"

	bs "backstage.io/backstage-operator/api/v1alpha1"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

const (
	_defaultBackstageInitContainerName = "install-dynamic-plugins"
	_defaultBackstageMainContainerName = "backstage-backend"
	_defaultBackendAuthSecretValue     = "pl4s3Ch4ng3M3"
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
        - name: dynamic-plugins-npmrc
          secret:
            defaultMode: 420
            optional: true
            secretName: dynamic-plugins-npmrc

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
            - mountPath: /opt/app-root/src/dynamic-plugins-root
              name: dynamic-plugins-root
`, _defaultBackstageMainContainerName)
)

type appConfigData struct {
	ref   string
	files []string
}

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
				if err = controllerutil.SetControllerReference(&backstage, deployment, r.Scheme); err != nil {
					return fmt.Errorf("failed to set owner reference: %s", err)
				}
			}

			var (
				backendAuthSecretName  string
				dpConf                 bs.DynamicPluginsConfigRef
				appConfigFilenamesList []appConfigData
			)
			backendAuthSecretName, err = r.handleBackendAuthSecret(ctx, backstage, ns)
			if err != nil {
				return err
			}

			dpConf, err = r.getOrGenerateDynamicPluginsConf(ctx, backstage, ns)
			if err != nil {
				return err
			}

			r.addVolumes(backstage, dpConf, deployment)

			appConfigFilenamesList, err = r.extractAppConfigFileNames(ctx, backstage, ns)
			if err != nil {
				return err
			}
			r.addVolumeMounts(deployment, dpConf, appConfigFilenamesList)
			r.addContainerArgs(deployment, appConfigFilenamesList)
			r.addEnvVars(backstage, deployment, backendAuthSecretName)

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

func (r *BackstageReconciler) getOrGenerateDynamicPluginsConf(ctx context.Context, backstage bs.Backstage, ns string) (config bs.DynamicPluginsConfigRef, err error) {
	if backstage.Spec.DynamicPluginsConfig.Name != "" {
		return backstage.Spec.DynamicPluginsConfig, nil
	}
	//Generate a default ConfigMap for dynamic plugins
	dpConfigName := fmt.Sprintf("%s-dynamic-plugins", backstage.Name)
	conf := bs.DynamicPluginsConfigRef{
		Name: dpConfigName,
		Kind: "ConfigMap",
	}
	cm := &v1.ConfigMap{
		TypeMeta: metav1.TypeMeta{
			Kind:       "v1",
			APIVersion: "ConfigMap",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      dpConfigName,
			Namespace: ns,
		},
	}
	err = r.Get(ctx, types.NamespacedName{Name: dpConfigName, Namespace: ns}, cm)
	if err != nil {
		if !errors.IsNotFound(err) {
			return bs.DynamicPluginsConfigRef{}, fmt.Errorf("failed to get config map for dynamic plugins (%q), reason: %s", dpConfigName, err)
		}
		cm.Data = map[string]string{
			"dynamic-plugins.yaml": `
includes:
- dynamic-plugins.default.yaml
plugins: []
`,
		}
		err = r.Create(ctx, cm)
		if err != nil {
			return bs.DynamicPluginsConfigRef{}, fmt.Errorf("failed to create config map for dynamic plugins, reason: %s", err)
		}
	}
	return conf, nil
}

func (r *BackstageReconciler) handleBackendAuthSecret(ctx context.Context, backstage bs.Backstage, ns string) (secretName string, err error) {
	backendAuthSecretName := backstage.Spec.BackendAuthSecretRef.Name
	if backendAuthSecretName == "" {
		//Generate a secret if it does not exist
		backendAuthSecretName = fmt.Sprintf("%s-auth", backstage.Name)
		sec := &v1.Secret{
			TypeMeta: metav1.TypeMeta{
				Kind:       "v1",
				APIVersion: "Secret",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      backendAuthSecretName,
				Namespace: ns,
			},
		}
		err = r.Get(ctx, types.NamespacedName{Name: backendAuthSecretName, Namespace: ns}, sec)
		if err != nil {
			if !errors.IsNotFound(err) {
				return "", fmt.Errorf("failed to get secret for backend auth (%q), reason: %s", backendAuthSecretName, err)
			}
			// Create a secret with a random value
			authVal := func(length int) string {
				bytes := make([]byte, length)
				if _, randErr := rand.Read(bytes); randErr != nil {
					// Do not fail, but use a fallback value
					return _defaultBackendAuthSecretValue
				}
				return base64.StdEncoding.EncodeToString(bytes)
			}(24)
			k := backstage.Spec.BackendAuthSecretRef.Key
			if k == "" {
				//TODO(rm3l): why kubebuilder default values do not work
				k = "backend-secret"
			}
			sec.Data = map[string][]byte{
				k: []byte(authVal),
			}
			err = r.Create(ctx, sec)
			if err != nil {
				return "", fmt.Errorf("failed to create secret for backend auth, reason: %s", err)
			}
		}
	}
	return backendAuthSecretName, nil
}

func (r *BackstageReconciler) addVolumes(backstage bs.Backstage, dynamicPluginsConf bs.DynamicPluginsConfigRef, deployment *appsv1.Deployment) {
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
	if dynamicPluginsConf.Name != "" {
		var volumeSource v1.VolumeSource
		switch dynamicPluginsConf.Kind {
		case "ConfigMap":
			volumeSource.ConfigMap = &v1.ConfigMapVolumeSource{
				DefaultMode:          pointer.Int32(420),
				LocalObjectReference: v1.LocalObjectReference{Name: dynamicPluginsConf.Name},
			}
		case "Secret":
			volumeSource.Secret = &v1.SecretVolumeSource{
				DefaultMode: pointer.Int32(420),
				SecretName:  dynamicPluginsConf.Name,
			}
		}
		deployment.Spec.Template.Spec.Volumes = append(deployment.Spec.Template.Spec.Volumes,
			v1.Volume{
				Name:         dynamicPluginsConf.Name,
				VolumeSource: volumeSource,
			})
	}
}

func (r *BackstageReconciler) addVolumeMounts(deployment *appsv1.Deployment, dynamicPluginsConf bs.DynamicPluginsConfigRef, appConfigFilenamesList []appConfigData) {
	if dynamicPluginsConf.Name != "" {
		for i, c := range deployment.Spec.Template.Spec.InitContainers {
			if c.Name == _defaultBackstageInitContainerName {
				deployment.Spec.Template.Spec.InitContainers[i].VolumeMounts = append(deployment.Spec.Template.Spec.InitContainers[i].VolumeMounts,
					v1.VolumeMount{
						Name:      dynamicPluginsConf.Name,
						MountPath: "/opt/app-root/src/dynamic-plugins.yaml",
						ReadOnly:  true,
						SubPath:   "dynamic-plugins.yaml",
					})
				break
			}
		}
	}

	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			for _, appConfigFilenames := range appConfigFilenamesList {
				deployment.Spec.Template.Spec.Containers[i].VolumeMounts = append(deployment.Spec.Template.Spec.Containers[i].VolumeMounts,
					v1.VolumeMount{
						Name:      appConfigFilenames.ref,
						MountPath: "/opt/app-root/src/" + appConfigFilenames.ref,
					})
			}
			break
		}
	}
}

func (r *BackstageReconciler) addContainerArgs(deployment *appsv1.Deployment, appConfigFilenamesList []appConfigData) {
	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			for _, appConfigFilenames := range appConfigFilenamesList {
				// Args
				for _, fileName := range appConfigFilenames.files {
					deployment.Spec.Template.Spec.Containers[i].Args =
						append(deployment.Spec.Template.Spec.Containers[i].Args, "--config",
							fmt.Sprintf("/opt/app-root/src/%s/%s", appConfigFilenames.ref, fileName))
				}
			}
			break
		}
	}
}

func (r *BackstageReconciler) addEnvVars(backstage bs.Backstage, deployment *appsv1.Deployment, backendAuthSecretName string) {
	if backendAuthSecretName == "" {
		return
	}
	for i, c := range deployment.Spec.Template.Spec.Containers {
		if c.Name == _defaultBackstageMainContainerName {
			k := backstage.Spec.BackendAuthSecretRef.Key
			if k == "" {
				//TODO(rm3l): why kubebuilder default values do not work
				k = "backend-secret"
			}
			deployment.Spec.Template.Spec.Containers[i].Env = append(deployment.Spec.Template.Spec.Containers[i].Env,
				v1.EnvVar{
					Name: "BACKEND_SECRET",
					ValueFrom: &v1.EnvVarSource{
						SecretKeyRef: &v1.SecretKeySelector{
							LocalObjectReference: v1.LocalObjectReference{
								Name: backendAuthSecretName,
							},
							Key:      k,
							Optional: pointer.Bool(false),
						},
					},
				},
				v1.EnvVar{
					Name:  "APP_CONFIG_backend_auth_keys",
					Value: `[{"secret": "$(BACKEND_SECRET)"}]`,
				})
			break
		}
	}
}

// extractAppConfigFileNames returns a mapping of app-config object name and the list of files in it.
// We intentionally do not return a Map, to preserve the iteration order of the AppConfigs in the Custom Resource,
// even though we can't guarantee the iteration order of the files listed inside each ConfigMap or Secret.
func (r *BackstageReconciler) extractAppConfigFileNames(ctx context.Context, backstage bs.Backstage, ns string) ([]appConfigData, error) {
	var result []appConfigData
	for _, appConfig := range backstage.Spec.AppConfigs {
		var files []string
		switch appConfig.Kind {
		case "ConfigMap":
			cm := v1.ConfigMap{}
			if err := r.Get(ctx, types.NamespacedName{Name: appConfig.Name, Namespace: ns}, &cm); err != nil {
				return nil, err
			}
			for filename := range cm.Data {
				// Bear in mind that iteration order over this map is not guaranteed by Go
				files = append(files, filename)
			}
			for filename := range cm.BinaryData {
				// Bear in mind that iteration order over this map is not guaranteed by Go
				files = append(files, filename)
			}
		case "Secret":
			sec := v1.Secret{}
			if err := r.Get(ctx, types.NamespacedName{Name: appConfig.Name, Namespace: ns}, &sec); err != nil {
				return nil, err
			}
			for filename := range sec.Data {
				// Bear in mind that iteration order over this map is not guaranteed by Go
				files = append(files, filename)
			}
			for filename := range sec.StringData {
				// Bear in mind that iteration order over this map is not guaranteed by Go
				files = append(files, filename)
			}
		}
		result = append(result, appConfigData{
			ref:   appConfig.Name,
			files: files,
		})
	}
	return result, nil
}
