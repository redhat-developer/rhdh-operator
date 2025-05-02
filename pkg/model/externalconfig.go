package model

import (
	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
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

	OpenShiftIngressDomain string

	WatchingHash string
}

func NewExternalConfig() ExternalConfig {
	__sealights__.TraceFunc("9a6f916c3e2260fefa")

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
	__sealights__.TraceFunc("a43e36a4b930301d62")
	return DataObjectKeys{
		StringDataKey: maps.Keys(stringData),
		BinaryDataKey: maps.Keys(binaryData),
	}
}

func (k DataObjectKeys) All() []string {
	__sealights__.TraceFunc("1a1c4a4a5eadafc442")
	return append(k.StringDataKey, k.BinaryDataKey...)
}
