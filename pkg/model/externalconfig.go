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
		DynamicPlugins:      corev1.ConfigMap{},

		syncedContent: []byte{},
	}
}

func (e *ExternalConfig) GetHash() string {
	h := sha256.New()
	h.Write([]byte(e.syncedContent))
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
