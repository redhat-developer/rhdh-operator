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
	RawConfig              map[string]string
	AppConfigs             map[string]corev1.ConfigMap
	ExtraFileConfigMaps    map[string]corev1.ConfigMap
	ExtraFileSecrets       map[string]corev1.Secret
	ExtraEnvConfigMaps     map[string]corev1.ConfigMap
	ExtraEnvSecrets        map[string]corev1.Secret
	DynamicPlugins         corev1.ConfigMap
	PersistentVolumeClaims map[string]corev1.PersistentVolumeClaim

	syncedContent []byte
}

func NewExternalConfig() ExternalConfig {

	return ExternalConfig{
		RawConfig:              map[string]string{},
		AppConfigs:             map[string]corev1.ConfigMap{},
		ExtraFileConfigMaps:    map[string]corev1.ConfigMap{},
		ExtraFileSecrets:       map[string]corev1.Secret{},
		ExtraEnvConfigMaps:     map[string]corev1.ConfigMap{},
		ExtraEnvSecrets:        map[string]corev1.Secret{},
		DynamicPlugins:         corev1.ConfigMap{},
		PersistentVolumeClaims: map[string]corev1.PersistentVolumeClaim{},

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
