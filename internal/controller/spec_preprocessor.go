package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"strconv"

	"golang.org/x/exp/maps"

	"k8s.io/client-go/util/retry"

	"k8s.io/apimachinery/pkg/api/errors"

	"sigs.k8s.io/controller-runtime/pkg/log"

	"sigs.k8s.io/controller-runtime/pkg/client"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	"github.com/redhat-developer/rhdh-operator/pkg/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

const AutoSyncEnvVar = "EXT_CONF_SYNC_backstage"

// Add additional details to the Backstage Spec helping in making Backstage RuntimeObjects Model
// Validates Backstage Spec and fails fast if something not correct
func (r *BackstageReconciler) preprocessSpec(ctx context.Context, backstage bsv1.Backstage) (model.ExternalConfig, error) {
	//lg := log.FromContext(ctx)

	bsSpec := backstage.Spec
	ns := backstage.Namespace

	result := model.NewExternalConfig()

	hashingData := []byte{}
	var err error

	// Process RawConfig
	if bsSpec.RawRuntimeConfig != nil {
		if bsSpec.RawRuntimeConfig.BackstageConfigName != "" {
			cm := &corev1.ConfigMap{}
			if err = r.checkExternalObject(ctx, cm, bsSpec.RawRuntimeConfig.BackstageConfigName, ns); err != nil {
				return result, err
			}
			for key, value := range cm.Data {
				result.RawConfig[key] = value
			}
		}
		if bsSpec.RawRuntimeConfig.LocalDbConfigName != "" {
			cm := &corev1.ConfigMap{}
			if err = r.checkExternalObject(ctx, cm, bsSpec.RawRuntimeConfig.LocalDbConfigName, ns); err != nil {
				return result, err
			}
			for key, value := range cm.Data {
				result.RawConfig[key] = value
			}
		}
	}

	if bsSpec.Application == nil {
		bsSpec.Application = &bsv1.Application{}
	}

	// Process AppConfigs
	if bsSpec.Application.AppConfig != nil {
		for _, ac := range bsSpec.Application.AppConfig.ConfigMaps {
			cm := &corev1.ConfigMap{Data: map[string]string{}}
			if hashingData, err = r.addExtConfig(&result, ctx, cm, backstage.Name, ac.Name, ns, addToWatch(ac), hashingData); err != nil {
				return result, err
			}
			result.AppConfigKeys[ac.Name] = maps.Keys(cm.Data)
		}
	}

	// Process ConfigMapFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.ConfigMaps != nil {
		for _, ef := range bsSpec.Application.ExtraFiles.ConfigMaps {
			cm := &corev1.ConfigMap{Data: map[string]string{}, BinaryData: map[string][]byte{}}
			if hashingData, err = r.addExtConfig(&result, ctx, cm, backstage.Name, ef.Name, ns, addToWatch(ef), hashingData); err != nil {
				return result, err
			}
			result.ExtraFileConfigMapKeys[ef.Name] = model.NewDataObjectKeys(cm.Data, cm.BinaryData)
		}
	}

	// Process SecretFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.Secrets != nil {
		for _, ef := range bsSpec.Application.ExtraFiles.Secrets {
			secret := &corev1.Secret{Data: map[string][]byte{}, StringData: map[string]string{}}
			if hashingData, err = r.addExtConfig(&result, ctx, secret, backstage.Name, ef.Name, ns, addToWatch(ef), hashingData); err != nil {
				return result, err
			}
			result.ExtraFileSecretKeys[ef.Name] = model.NewDataObjectKeys(secret.StringData, secret.Data)
		}
	}

	// Process ConfigMapEnvs
	if bsSpec.Application.ExtraEnvs != nil && bsSpec.Application.ExtraEnvs.ConfigMaps != nil {
		for _, ee := range bsSpec.Application.ExtraEnvs.ConfigMaps {
			cm := &corev1.ConfigMap{Data: map[string]string{}, BinaryData: map[string][]byte{}}
			if hashingData, err = r.addExtConfig(&result, ctx, cm, backstage.Name, ee.Name, ns, true, hashingData); err != nil {
				return result, err
			}
			result.ExtraEnvConfigMapKeys[ee.Name] = model.NewDataObjectKeys(cm.Data, cm.BinaryData)
		}
	}

	// Process SecretEnvs
	if bsSpec.Application.ExtraEnvs != nil && bsSpec.Application.ExtraEnvs.Secrets != nil {
		for _, ee := range bsSpec.Application.ExtraEnvs.Secrets {
			secret := &corev1.Secret{Data: map[string][]byte{}, StringData: map[string]string{}}
			if hashingData, err = r.addExtConfig(&result, ctx, secret, backstage.Name, ee.Name, ns, true, hashingData); err != nil {
				return result, err
			}
			//result.ExtraEnvSecrets[secret.Name] = *secret
			result.ExtraEnvSecretKeys[ee.Name] = model.NewDataObjectKeys(secret.StringData, secret.Data)
		}
	}

	// Process PVCFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.Pvcs != nil {
		for _, ep := range bsSpec.Application.ExtraFiles.Pvcs {
			pvc := &corev1.PersistentVolumeClaim{}
			if err := r.checkExternalObject(ctx, pvc, ep.Name, ns); err != nil {
				return result, err
			}
			//result.ExtraPvcs[pvc.Name] = *pvc
			result.ExtraPvcKeys = append(result.ExtraPvcKeys, pvc.Name)
		}
	}

	// Process DynamicPlugins
	if bsSpec.Application.DynamicPluginsConfigMapName != "" {
		cm := &corev1.ConfigMap{}
		if hashingData, err = r.addExtConfig(&result, ctx, cm, backstage.Name, bsSpec.Application.DynamicPluginsConfigMapName, ns, true, hashingData); err != nil {
			return result, err
		}
		result.DynamicPlugins = *cm
	}

	hash := sha256.New()
	hash.Write(hashingData)
	result.WatchingHash = fmt.Sprintf("%x", hash.Sum(nil))

	return result, nil
}

// addExtConfig makes object watchable by Operator adding ExtConfigSyncLabel label and BackstageNameAnnotation
// and adding its content (marshalled object) to make it watchable by Operator and able to refresh the Pod if needed
// (Pod refresh will be called if external configuration hash changed)
func (r *BackstageReconciler) addExtConfig(config *model.ExternalConfig, ctx context.Context, obj client.Object, backstageName, objectName, ns string, addToWatch bool, hashingData []byte) ([]byte, error) {

	lg := log.FromContext(ctx)

	if !addToWatch {
		return hashingData, r.checkExternalObject(ctx, obj, objectName, ns)
	}

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

		return nil

	})

	return concatData(hashingData, obj), err
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

func addToWatch(fileObjectRef bsv1.FileObjectRef) bool {
	// it will contain subPath either as specified key or as a list of all keys if only mountPath specified
	if fileObjectRef.MountPath == "" || fileObjectRef.Key != "" {
		return true
	}
	return false
}

func concatData(original []byte, obj client.Object) []byte {
	data := original
	stringData := map[string]string{}
	binaryData := map[string][]byte{}

	switch v := obj.(type) {
	case *corev1.ConfigMap:
		stringData = v.Data
		binaryData = v.BinaryData
	case *corev1.Secret:
		stringData = v.StringData
		binaryData = v.Data
	}

	for k, v := range stringData {
		data = append(data, []byte(k+v)...)
	}

	for k, v := range binaryData {
		data = append(data, []byte(k)...)
		data = append(data, v...)
	}

	return data
}
