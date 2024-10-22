package controller

import (
	"context"
	"fmt"
	"os"
	"strconv"

	"k8s.io/client-go/util/retry"

	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	bs "redhat-developer/red-hat-developer-hub-operator/api/v1alpha3"

	"redhat-developer/red-hat-developer-hub-operator/pkg/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const AutoSyncEnvVar = "EXT_CONF_SYNC_backstage"

// Add additional details to the Backstage Spec helping in making Backstage RuntimeObjects Model
// Validates Backstage Spec and fails fast if something not correct
func (r *BackstageReconciler) preprocessSpec(ctx context.Context, backstage bs.Backstage) (model.ExternalConfig, error) {
	//lg := log.FromContext(ctx)

	bsSpec := backstage.Spec
	ns := backstage.Namespace

	result := model.NewExternalConfig()

	// Process RawConfig
	if bsSpec.RawRuntimeConfig != nil {
		if bsSpec.RawRuntimeConfig.BackstageConfigName != "" {
			cm := &corev1.ConfigMap{}
			if err := r.checkExternalObject(ctx, cm, bsSpec.RawRuntimeConfig.BackstageConfigName, ns); err != nil {
				return result, err
			}
			for key, value := range cm.Data {
				result.RawConfig[key] = value
			}
		}
		if bsSpec.RawRuntimeConfig.LocalDbConfigName != "" {
			cm := &corev1.ConfigMap{}
			if err := r.checkExternalObject(ctx, cm, bsSpec.RawRuntimeConfig.LocalDbConfigName, ns); err != nil {
				return result, err
			}
			for key, value := range cm.Data {
				result.RawConfig[key] = value
			}
		}
	}

	if bsSpec.Application == nil {
		bsSpec.Application = &bs.Application{}
	}

	// Process AppConfigs
	if bsSpec.Application.AppConfig != nil {
		for _, ac := range bsSpec.Application.AppConfig.ConfigMaps {
			cm := &corev1.ConfigMap{}
			if err := r.addExtConfig(&result, ctx, cm, backstage.Name, ac.Name, ns); err != nil {
				return result, err
			}
			result.AppConfigs[ac.Name] = *cm
		}
	}

	// Process ConfigMapFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.ConfigMaps != nil {
		for _, ef := range bsSpec.Application.ExtraFiles.ConfigMaps {
			cm := &corev1.ConfigMap{}
			if err := r.addExtConfig(&result, ctx, cm, backstage.Name, ef.Name, ns); err != nil {
				return result, err
			}
			result.ExtraFileConfigMaps[cm.Name] = *cm
		}
	}

	// Process SecretFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.Secrets != nil {
		for _, ef := range bsSpec.Application.ExtraFiles.Secrets {
			secret := &corev1.Secret{}
			if err := r.addExtConfig(&result, ctx, secret, backstage.Name, ef.Name, ns); err != nil {
				return result, err
			}
			result.ExtraFileSecrets[secret.Name] = *secret
		}
	}

	// Process ConfigMapEnvs
	if bsSpec.Application.ExtraEnvs != nil && bsSpec.Application.ExtraEnvs.ConfigMaps != nil {
		for _, ee := range bsSpec.Application.ExtraEnvs.ConfigMaps {
			cm := &corev1.ConfigMap{}
			if err := r.addExtConfig(&result, ctx, cm, backstage.Name, ee.Name, ns); err != nil {
				return result, err
			}
			result.ExtraEnvConfigMaps[cm.Name] = *cm
		}
	}

	// Process SecretEnvs
	if bsSpec.Application.ExtraEnvs != nil && bsSpec.Application.ExtraEnvs.Secrets != nil {
		for _, ee := range bsSpec.Application.ExtraEnvs.Secrets {
			secret := &corev1.Secret{}
			if err := r.addExtConfig(&result, ctx, secret, backstage.Name, ee.Name, ns); err != nil {
				return result, err
			}
			result.ExtraEnvSecrets[secret.Name] = *secret
		}
	}

	// Process PVCFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.Pvcs != nil {
		for _, ep := range bsSpec.Application.ExtraFiles.Pvcs {
			pvc := &corev1.PersistentVolumeClaim{}
			if err := r.checkExternalObject(ctx, pvc, ep.Name, ns); err != nil {
				return result, err
			}
			result.ExtraPvcs[pvc.Name] = *pvc
		}
	}

	// Process DynamicPlugins
	if bsSpec.Application.DynamicPluginsConfigMapName != "" {
		cm := &corev1.ConfigMap{}
		if err := r.addExtConfig(&result, ctx, cm, backstage.Name, bsSpec.Application.DynamicPluginsConfigMapName, ns); err != nil {
			return result, err
		}
		result.DynamicPlugins = *cm
	}

	return result, nil
}

func (r *BackstageReconciler) addExtConfig(config *model.ExternalConfig, ctx context.Context, obj client.Object, backstageName, objectName, ns string) error {

	lg := log.FromContext(ctx)

	// use RetryOnConflict to avoid possible Conflict error which may be caused mostly by other call of this function
	// https://pkg.go.dev/k8s.io/client-go/util/retry#RetryOnConflict.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {

		if err := r.checkExternalObject(ctx, obj, objectName, ns); err != nil {
			return err
		}

		if obj.GetLabels() == nil {
			obj.SetLabels(map[string]string{})
		}
		if obj.GetAnnotations() == nil {
			obj.SetAnnotations(map[string]string{})
		}

		autoSync := true
		autoSyncStr, ok := os.LookupEnv(AutoSyncEnvVar)
		if ok {
			autoSync, _ = strconv.ParseBool(autoSyncStr)
		}

		if obj.GetLabels()[model.ExtConfigSyncLabel] == "" || obj.GetAnnotations()[model.BackstageNameAnnotation] == "" ||
			obj.GetLabels()[model.ExtConfigSyncLabel] != strconv.FormatBool(autoSync) {

			obj.GetLabels()[model.ExtConfigSyncLabel] = strconv.FormatBool(autoSync)
			obj.GetAnnotations()[model.BackstageNameAnnotation] = backstageName
			if err := r.Update(ctx, obj); err != nil {
				return err
			}
			lg.V(1).Info(fmt.Sprintf("update external config %s with label %s=%s and annotation %s=%s", obj.GetName(), model.ExtConfigSyncLabel, strconv.FormatBool(autoSync), model.BackstageNameAnnotation, backstageName))
		}

		if err := config.AddToSyncedConfig(obj); err != nil {
			return fmt.Errorf("failed to add to synced %s: %s", obj.GetName(), err)
		}

		return nil

	})
	return err
}

func (r *BackstageReconciler) checkExternalObject(ctx context.Context, obj client.Object, objectName, ns string) error {
	if err := r.Get(ctx, types.NamespacedName{Name: objectName, Namespace: ns}, obj); err != nil {
		if _, ok := obj.(*corev1.Secret); ok && errors.IsForbidden(err) {
			return fmt.Errorf("warning: Secrets GET is forbidden, updating Secrets may not cause Pod recreating")
		}
		return fmt.Errorf("failed to get external config from %s: %s", objectName, err)
	}
	return nil
}
