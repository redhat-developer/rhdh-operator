package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const ExtConfigSyncLabel = "rhdh.redhat.com/ext-config-sync"
const BackstageNameAnnotation = "rhdh.redhat.com/backstage-name"

type ExternalConfig struct {
	RawConfig           map[string]string
	AppConfigs          map[string]corev1.ConfigMap
	ExtraFileConfigMaps map[string]corev1.ConfigMap
	ExtraFileSecrets    map[string]corev1.Secret
	ExtraEnvConfigMaps  map[string]corev1.ConfigMap
	ExtraEnvSecrets     map[string]corev1.Secret
	ExtraPvcs           map[string]corev1.PersistentVolumeClaim
	DynamicPlugins      corev1.ConfigMap

	syncedContent []byte
}

func NewExternalConfig() ExternalConfig {

	return ExternalConfig{
		RawConfig:           map[string]string{},
		AppConfigs:          map[string]corev1.ConfigMap{},
		ExtraFileConfigMaps: map[string]corev1.ConfigMap{},
		ExtraFileSecrets:    map[string]corev1.Secret{},
		ExtraEnvConfigMaps:  map[string]corev1.ConfigMap{},
		ExtraEnvSecrets:     map[string]corev1.Secret{},
		ExtraPvcs:           map[string]corev1.PersistentVolumeClaim{},
		DynamicPlugins:      corev1.ConfigMap{},

		syncedContent: []byte{},
	}
}

func (e *ExternalConfig) GetHash() string {
	h := sha256.New()
	h.Write(e.syncedContent)
	h.Reset()
	return fmt.Sprintf("%x", h.Sum(nil))
}

func (e *ExternalConfig) AddToSyncedConfig(content client.Object) error {

	d, err := json.Marshal(content)
	if err != nil {
		return err
	}

	e.syncedContent = append(e.syncedContent, d...)
	return nil
}

//func (e *ExternalConfig) AddToSyncedConfig(obj client.Object) error {
//
//	stringData := map[string]string{}
//	binaryData := map[string][]byte{}
//
//	switch v := obj.(type) {
//	case *corev1.ConfigMap:
//		stringData = v.Data
//		binaryData = v.BinaryData
//	case *corev1.Secret:
//		stringData = v.StringData
//		binaryData = v.Data
//	default:
//		return fmt.Errorf("unsupported value type: %v", v)
//	}
//
//	for k, v := range stringData {
//		e.syncedContent = append(e.syncedContent, []byte(k+v)...)
//	}
//
//	for k, v := range binaryData {
//		e.syncedContent = append(e.syncedContent, []byte(k)...)
//		e.syncedContent = append(e.syncedContent, v...)
//	}
//	return nil
//}

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
		//default:
		//	return fmt.Errorf("unsupported value type: %v", v)
	}

	for k, v := range stringData {
		//h.Sum([]byte(k + v))
		data = append(data, []byte(k+v)...)
	}

	for k, v := range binaryData {
		//h.Sum([]byte(k))
		//h.Sum(v)
		data = append(data, []byte(k)...)
		data = append(data, v...)
	}
	return data
}
