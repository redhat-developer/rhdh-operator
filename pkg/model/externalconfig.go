package model

import (
	"golang.org/x/exp/maps"
	corev1 "k8s.io/api/core/v1"
)

const ExtConfigSyncLabel = "rhdh.redhat.com/ext-config-sync"
const BackstageNameAnnotation = "rhdh.redhat.com/backstage-name"

type ExternalConfig struct {
	RawConfig              map[string]string
	DynamicPlugins         corev1.ConfigMap
	AppConfigKeys          map[string][]string
	ExtraFileConfigMapKeys map[string]DataObjectKeys
	ExtraFileSecretKeys    map[string]DataObjectKeys
	ExtraEnvConfigMapKeys  map[string]DataObjectKeys
	ExtraEnvSecretKeys     map[string]DataObjectKeys
	ExtraPvcKeys           []string

	WatchingHash string
}

func NewExternalConfig() ExternalConfig {

	return ExternalConfig{
		RawConfig:              map[string]string{},
		DynamicPlugins:         corev1.ConfigMap{},
		AppConfigKeys:          map[string][]string{},
		ExtraFileConfigMapKeys: map[string]DataObjectKeys{},
		ExtraFileSecretKeys:    map[string]DataObjectKeys{},
		ExtraEnvConfigMapKeys:  map[string]DataObjectKeys{},
		ExtraEnvSecretKeys:     map[string]DataObjectKeys{},
		ExtraPvcKeys:           []string{},

		WatchingHash: "",
	}
}

type DataObjectKeys struct {
	StringDataKey []string
	BinaryDataKey []string
}

func NewDataObjectKeys(stringData map[string]string, binaryData map[string][]byte) DataObjectKeys {
	return DataObjectKeys{
		StringDataKey: maps.Keys(stringData),
		BinaryDataKey: maps.Keys(binaryData),
	}
}

func (k DataObjectKeys) All() []string {
	return append(k.StringDataKey, k.BinaryDataKey...)
}
