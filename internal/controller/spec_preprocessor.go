package controller

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"sort"
	"strconv"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/api/errors"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/util/retry"
	"k8s.io/klog/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/model"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
)

// Add additional details to the Backstage Spec helping in making Backstage RuntimeObjects Model
// Validates Backstage Spec and fails fast if something not correct
func (r *BackstageReconciler) preprocessSpec(ctx context.Context, backstage bsv1.Backstage) (model.ExternalConfig, error) {
	__sealights__.TraceFunc("ed58226a5acff9aa62")
	//lg := log.FromContext(ctx)

	bsSpec := backstage.Spec
	ns := backstage.Namespace

	result := model.NewExternalConfig()
	if r.Platform.IsOpenshift() {
		domain, err := r.getOCPIngressDomain()
		if err != nil {
			return result, err
		}
		result.OpenShiftIngressDomain = domain
	}

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
			if hashingData, err = r.addExtConfig(ctx, cm, backstage.Name, ac.Name, ns, addToWatch(ac), hashingData); err != nil {
				return result, err
			}
			result.AppConfigKeys[ac.Name] = maps.Keys(cm.Data)
		}
	}

	// Process ConfigMapFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.ConfigMaps != nil {
		for _, ef := range bsSpec.Application.ExtraFiles.ConfigMaps {
			cm := &corev1.ConfigMap{Data: map[string]string{}, BinaryData: map[string][]byte{}}
			if hashingData, err = r.addExtConfig(ctx, cm, backstage.Name, ef.Name, ns, addToWatch(ef), hashingData); err != nil {
				return result, err
			}
			result.ExtraFileConfigMapKeys[ef.Name] = model.NewDataObjectKeys(cm.Data, cm.BinaryData)
		}
	}

	// Process SecretFiles
	if bsSpec.Application.ExtraFiles != nil && bsSpec.Application.ExtraFiles.Secrets != nil {
		for _, ef := range bsSpec.Application.ExtraFiles.Secrets {
			secret := &corev1.Secret{Data: map[string][]byte{}, StringData: map[string]string{}}
			if hashingData, err = r.addExtConfig(ctx, secret, backstage.Name, ef.Name, ns, addToWatch(ef), hashingData); err != nil {
				return result, err
			}
			result.ExtraFileSecretKeys[ef.Name] = model.NewDataObjectKeys(secret.StringData, secret.Data)
		}
	}

	// Process ConfigMapEnvs
	if bsSpec.Application.ExtraEnvs != nil && bsSpec.Application.ExtraEnvs.ConfigMaps != nil {
		for _, ee := range bsSpec.Application.ExtraEnvs.ConfigMaps {
			cm := &corev1.ConfigMap{Data: map[string]string{}, BinaryData: map[string][]byte{}}
			if hashingData, err = r.addExtConfig(ctx, cm, backstage.Name, ee.Name, ns, true, hashingData); err != nil {
				return result, err
			}
			result.ExtraEnvConfigMapKeys[ee.Name] = model.NewDataObjectKeys(cm.Data, cm.BinaryData)
		}
	}

	// Process SecretEnvs
	if bsSpec.Application.ExtraEnvs != nil && bsSpec.Application.ExtraEnvs.Secrets != nil {
		for _, ee := range bsSpec.Application.ExtraEnvs.Secrets {
			secret := &corev1.Secret{Data: map[string][]byte{}, StringData: map[string]string{}}
			if hashingData, err = r.addExtConfig(ctx, secret, backstage.Name, ee.Name, ns, true, hashingData); err != nil {
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
		if hashingData, err = r.addExtConfig(ctx, cm, backstage.Name, bsSpec.Application.DynamicPluginsConfigMapName, ns, true, hashingData); err != nil {
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
func (r *BackstageReconciler) addExtConfig(ctx context.Context, obj client.Object, backstageName, objectName, ns string, addToWatch bool, hashingData []byte) ([]byte, error) {
	__sealights__.TraceFunc("f681f9c5369dd75364")

	lg := log.FromContext(ctx)

	if !addToWatch {
		return hashingData, r.checkExternalObject(ctx, obj, objectName, ns)
	}

	// use RetryOnConflict to avoid possible Conflict error which may be caused mostly by other call of this function
	// https://pkg.go.dev/k8s.io/client-go/util/retry#RetryOnConflict.
	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		__sealights__.TraceFunc("df82c5b2fa97b64642")

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
	__sealights__.TraceFunc("ed1fec5355edc03d87")
	if err := r.Get(ctx, types.NamespacedName{Name: objectName, Namespace: ns}, obj); err != nil {
		if _, ok := obj.(*corev1.Secret); ok && errors.IsForbidden(err) {
			return fmt.Errorf("warning: Secrets GET is forbidden, updating Secrets may not cause Pod recreating")
		}
		return fmt.Errorf("failed to get external config from %s: %s", objectName, err)
	}
	return nil
}

func addToWatch(fileObjectRef bsv1.FileObjectRef) bool {
	__sealights__.TraceFunc("6cd2bbfd66c07da89d")
	// it will contain subPath either as specified key or as a list of all keys if only mountPath specified
	if (fileObjectRef.MountPath == "" || fileObjectRef.Key != "") && utils.BoolEnvVar(WatchExtConfig, true) {
		return true
	}
	return false
}

func concatData(original []byte, obj client.Object) []byte {
	__sealights__.TraceFunc("6f2b3277fc33c2e13b")
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

	// sort the keys to make sure the order and the hash eventually is consistent
	stringKeys := make([]string, 0, len(stringData))
	for k := range stringData {
		stringKeys = append(stringKeys, k)
	}
	sort.Strings(stringKeys)

	binaryKeys := make([]string, 0, len(binaryData))
	for k := range binaryData {
		binaryKeys = append(binaryKeys, k)
	}
	sort.Strings(binaryKeys)

	// append the data to the original data
	for _, k := range stringKeys {
		data = append(data, []byte(k+stringData[k])...)
	}

	for _, k := range binaryKeys {
		data = append(data, []byte(k)...)
		data = append(data, binaryData[k]...)
	}

	return data
}

// getOCPIngressDomain returns the OpenShift Ingress domain
func (r *BackstageReconciler) getOCPIngressDomain() (string, error) {
	__sealights__.TraceFunc("afd3d2ff0a6f8faeb9")
	var u unstructured.Unstructured
	u.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "config.openshift.io",
		Kind:    "Ingress",
		Version: "v1",
	})

	err := r.Client.Get(context.Background(), client.ObjectKey{
		Name:      "cluster",
		Namespace: "",
	}, &u)
	if err != nil {
		if k8serrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			klog.V(1).Info("no cluster ingress config found")
			return "", nil
		}
		return "", err
	}

	d, ok, err := unstructured.NestedString(u.Object, "spec", "domain")
	if err != nil {
		return "", err
	}
	if !ok {
		klog.V(1).Info("spec.domain in Ingress cluster config not found")
		return "", nil
	}
	return d, nil
}
