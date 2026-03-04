package model

import (
	"os"
	"path/filepath"
	"testing"

	"gopkg.in/yaml.v2"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeDynamicPluginsFunction(t *testing.T) {
	scheme := runtime.NewScheme()
	err := corev1.AddToScheme(scheme)
	require.NoError(t, err)

	tests := []struct {
		name        string
		paths       []string
		wantErr     bool
		errContains string
		validate    func(t *testing.T, objs []runtime.Object)
	}{
		{
			name:    "empty paths returns empty array",
			paths:   []string{},
			wantErr: false,
			validate: func(t *testing.T, objs []runtime.Object) {
				assert.Empty(t, objs)
			},
		},
		{
			name:    "single path returns as-is",
			paths:   []string{"testdata/dynamic-plugins-base.yaml"},
			wantErr: false,
			validate: func(t *testing.T, objs []runtime.Object) {
				require.Len(t, objs, 1)
				cm := objs[0].(*corev1.ConfigMap)
				assert.Contains(t, cm.Data[DynamicPluginsFile], "plugin-a")
				assert.Contains(t, cm.Data[DynamicPluginsFile], "plugin-b")
			},
		},
		{
			name: "merge two paths - plugins by package name",
			paths: []string{
				"testdata/dynamic-plugins-base.yaml",
				"testdata/dynamic-plugins-overlay.yaml",
			},
			wantErr: false,
			validate: func(t *testing.T, objs []runtime.Object) {
				require.Len(t, objs, 1)
				cm := objs[0].(*corev1.ConfigMap)

				// Parse the merged data
				var config DynaPluginsConfig
				err := yaml.Unmarshal([]byte(cm.Data[DynamicPluginsFile]), &config)
				require.NoError(t, err)

				// Should have 3 plugins (a, b, c)
				assert.Len(t, config.Plugins, 3)

				// Find plugin-b to verify it was overridden
				var pluginB *DynaPlugin
				for i := range config.Plugins {
					if config.Plugins[i].Package == "plugin-b" {
						pluginB = &config.Plugins[i]
						break
					}
				}
				require.NotNil(t, pluginB, "plugin-b should exist")
				assert.False(t, pluginB.Disabled, "plugin-b should be enabled (overridden)")
				assert.Equal(t, "sha512-overlay", pluginB.Integrity, "plugin-b integrity should be from overlay")

				// Includes should be merged
				assert.Len(t, config.Includes, 2)
				includesMap := make(map[string]bool)
				for _, inc := range config.Includes {
					includesMap[inc] = true
				}
				assert.True(t, includesMap["dynamic-plugins.default.yaml"])
				assert.True(t, includesMap["dynamic-plugins.custom.yaml"])
			},
		},
		{
			name: "nil configmap - file contains non-ConfigMap object",
			paths: []string{
				"testdata/not-a-configmap.yaml",
			},
			wantErr:     true,
			errContains: "dynamic-plugins.yaml",
		},
		{
			name: "nil configmap in merge - second file has no ConfigMap",
			paths: []string{
				"testdata/dynamic-plugins-base.yaml",
				"testdata/not-a-configmap.yaml",
			},
			wantErr:     true,
			errContains: "dynamic-plugins.yaml",
		},
		{
			name: "non-existent file",
			paths: []string{
				"testdata/does-not-exist.yaml",
			},
			wantErr:     true,
			errContains: "failed to read",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			objs, err := mergeDynamicPlugins(tt.paths, *scheme, "")

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)
				if tt.validate != nil {
					// Convert []client.Object to []runtime.Object for validation
					runtimeObjs := make([]runtime.Object, len(objs))
					for i, obj := range objs {
						runtimeObjs[i] = obj
					}
					tt.validate(t, runtimeObjs)
				}
			}
		})
	}
}

func TestGetEnabledFlavours(t *testing.T) {
	// Setup: Set LOCALBIN to testdata directory for tests
	originalLocalBin := os.Getenv("LOCALBIN")
	testDataDir, err := filepath.Abs("testdata")
	require.NoError(t, err)
	err = os.Setenv("LOCALBIN", testDataDir)
	require.NoError(t, err)
	defer func() {
		_ = os.Setenv("LOCALBIN", originalLocalBin)
	}()

	tests := []struct {
		name         string
		spec         api.BackstageSpec
		wantFlavours []string // expected flavour names in order
		wantErr      bool
		errContains  string
	}{
		{
			name:         "nil spec.Flavours returns all defaults",
			spec:         api.BackstageSpec{Flavours: nil},
			wantFlavours: []string{"lightspeed", "custom"},
			wantErr:      false,
		},
		{
			name: "empty spec.Flavours returns defaults not mentioned",
			spec: api.BackstageSpec{
				Flavours: []api.Flavour{},
			},
			wantFlavours: []string{"lightspeed", "custom"},
			wantErr:      false,
		},
		{
			name: "explicit enabled flavour",
			spec: api.BackstageSpec{
				Flavours: []api.Flavour{
					{Name: "orchestrator", Enabled: true},
				},
			},
			// orchestrator (explicit) + defaults (lightspeed, custom)
			wantFlavours: []string{"orchestrator", "lightspeed", "custom"},
			wantErr:      false,
		},
		{
			name: "default enabled flavour disabled explicitly",
			spec: api.BackstageSpec{
				Flavours: []api.Flavour{
					{Name: "lightspeed", Enabled: false},
				},
			},
			// lightspeed disabled, only custom (default) remains
			wantFlavours: []string{"custom"},
			wantErr:      false,
		},
		{
			name: "default disabled flavour enabled explicitly",
			spec: api.BackstageSpec{
				Flavours: []api.Flavour{
					{Name: "orchestrator", Enabled: true},
				},
			},
			// orchestrator (explicit enabled) + defaults (lightspeed, custom)
			wantFlavours: []string{"orchestrator", "lightspeed", "custom"},
			wantErr:      false,
		},
		{
			name: "mix of explicit enabled, disabled, and defaults",
			spec: api.BackstageSpec{
				Flavours: []api.Flavour{
					{Name: "orchestrator", Enabled: true}, // default=false, spec=enabled
					{Name: "lightspeed", Enabled: false},  // default=true, spec=disabled
					{Name: "custom", Enabled: true},       // default=true, spec=enabled
				},
			},
			// orchestrator (explicit), custom (explicit), no lightspeed (disabled)
			wantFlavours: []string{"orchestrator", "custom"},
			wantErr:      false,
		},
		{
			name: "spec order is preserved for explicit flavours",
			spec: api.BackstageSpec{
				Flavours: []api.Flavour{
					{Name: "custom", Enabled: true},
					{Name: "orchestrator", Enabled: true},
					{Name: "lightspeed", Enabled: true},
				},
			},
			// Should be in spec order since all are explicit
			wantFlavours: []string{"custom", "orchestrator", "lightspeed"},
			wantErr:      false,
		},
		{
			name: "nonexistent flavour returns error",
			spec: api.BackstageSpec{
				Flavours: []api.Flavour{
					{Name: "nonexistent", Enabled: true},
				},
			},
			wantErr:     true,
			errContains: "flavour 'nonexistent' not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			flavours, err := GetEnabledFlavours(tt.spec)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.errContains != "" {
					assert.Contains(t, err.Error(), tt.errContains)
				}
			} else {
				require.NoError(t, err)

				// Extract flavour names
				gotNames := make([]string, len(flavours))
				for i, f := range flavours {
					gotNames[i] = f.name
				}

				// Assert exact match (order matters)
				if !assert.ElementsMatch(t, tt.wantFlavours, gotNames) {
					t.Logf("Expected flavours: %v", tt.wantFlavours)
					t.Logf("Got flavours: %v", gotNames)
				}
			}
		})
	}
}
