package model

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

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

// GetMountPath calculates mount path and whether the volume will be mounted with subPath by specified ObjectKeyRef and spec.app.extrafiles.mountPath using the following rules:
// If only mountPath specified in the ObjectKeyRef - mount to it as directory w/o subPath (wSubpath == false)
// If key specified in the ObjectKeyRef - mount the file specified as a key with subpath (wSubpath == true) to ObjectKeyRef.mountPath if specified or to default
// If nothing specified in the ObjectKeyRef - mount to default file-by-file with subpath (wSubpath == true)
func GetMountPath(objectRef bsv1.FileObjectKeyRef, extraFilesMountPath string) (string, bool) {

	mp := DefaultMountDir
	if extraFilesMountPath != "" {
		mp = extraFilesMountPath
	}

	wSubpath := true
	if objectRef.MountPath != "" {
		if filepath.IsAbs(objectRef.MountPath) {
			mp = objectRef.MountPath
		} else {
			mp = filepath.Join(mp, objectRef.MountPath)
		}

		if objectRef.Key == "" {
			wSubpath = false
		}
	}

	return mp, wSubpath
}
