package controller

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/catalog"
	"github.com/redhat-developer/rhdh-operator/pkg/model"
)

func setupCatalogTestReconciler(objects ...client.Object) *DevHubPluginCatalogReconciler {
	scheme := runtime.NewScheme()
	_ = api.AddToScheme(scheme)
	_ = corev1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objects...).
		Build()

	return &DevHubPluginCatalogReconciler{
		Client:            fakeClient,
		Scheme:            scheme,
		OperatorNamespace: "rhdh-operator",
		Processor:         catalog.NewProcessor(),
	}
}

// listAndBuildInputs is a test helper that lists catalogs and builds inputs
func listAndBuildInputs(r *DevHubPluginCatalogReconciler, ctx context.Context) ([]catalog.CatalogInput, error) {
	catalogList := &api.DevHubPluginCatalogList{}
	if err := r.List(ctx, catalogList); err != nil {
		return nil, err
	}
	return r.buildCatalogInputs(ctx, catalogList)
}

func TestBuildCatalogInputs_NoCatalogs(t *testing.T) {
	r := setupCatalogTestReconciler()

	inputs, err := listAndBuildInputs(r, context.TODO())
	require.NoError(t, err)
	assert.Empty(t, inputs)
}

func TestBuildCatalogInputs_SingleCatalog(t *testing.T) {
	dhpc := &api.DevHubPluginCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-catalog",
		},
		Spec: api.DevHubPluginCatalogSpec{
			Source: api.CatalogSource{
				Ref: "oci://registry.example.com/rhdh/plugin-catalog:v1",
			},
		},
	}

	r := setupCatalogTestReconciler(dhpc)

	inputs, err := listAndBuildInputs(r, context.TODO())
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	assert.Equal(t, "oci://registry.example.com/rhdh/plugin-catalog:v1", inputs[0].Ref)
	assert.False(t, inputs[0].SkipTLSVerify)
	assert.Nil(t, inputs[0].DockerConfig)
	assert.Nil(t, inputs[0].CACert)
}

func TestBuildCatalogInputs_WithSkipTLSVerify(t *testing.T) {
	dhpc := &api.DevHubPluginCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-catalog",
		},
		Spec: api.DevHubPluginCatalogSpec{
			Source: api.CatalogSource{
				Ref:           "oci://registry.example.com/rhdh/plugin-catalog:v1",
				SkipTLSVerify: true,
			},
		},
	}

	r := setupCatalogTestReconciler(dhpc)

	inputs, err := listAndBuildInputs(r, context.TODO())
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	assert.True(t, inputs[0].SkipTLSVerify)
}

func TestBuildCatalogInputs_WithPullSecret(t *testing.T) {
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry-creds",
			Namespace: "rhdh-operator",
		},
		Data: map[string][]byte{
			".dockerconfigjson": []byte(`{"auths":{"registry.example.com":{"auth":"dXNlcjpwYXNz"}}}`),
		},
	}

	dhpc := &api.DevHubPluginCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-catalog",
		},
		Spec: api.DevHubPluginCatalogSpec{
			Source: api.CatalogSource{
				Ref: "oci://registry.example.com/rhdh/plugin-catalog:v1",
				PullSecret: &corev1.LocalObjectReference{
					Name: "registry-creds",
				},
			},
		},
	}

	r := setupCatalogTestReconciler(dhpc, secret)

	inputs, err := listAndBuildInputs(r, context.TODO())
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	assert.NotNil(t, inputs[0].DockerConfig)
	assert.Contains(t, string(inputs[0].DockerConfig), "registry.example.com")
}

func TestBuildCatalogInputs_WithCACert(t *testing.T) {
	caCert := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpegPjMCMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
-----END CERTIFICATE-----`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry-ca",
			Namespace: "rhdh-operator",
		},
		Data: map[string]string{
			"ca.crt": caCert,
		},
	}

	dhpc := &api.DevHubPluginCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-catalog",
		},
		Spec: api.DevHubPluginCatalogSpec{
			Source: api.CatalogSource{
				Ref: "oci://registry.example.com/rhdh/plugin-catalog:v1",
				CertificateAuthority: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "registry-ca",
					},
					Key: "ca.crt",
				},
			},
		},
	}

	r := setupCatalogTestReconciler(dhpc, cm)

	inputs, err := listAndBuildInputs(r, context.TODO())
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	assert.NotNil(t, inputs[0].CACert)
	assert.Contains(t, string(inputs[0].CACert), "BEGIN CERTIFICATE")
}

func TestBuildCatalogInputs_WithCACert_DefaultKey(t *testing.T) {
	caCert := `-----BEGIN CERTIFICATE-----
MIIBkTCB+wIJAKHBfpegPjMCMA0GCSqGSIb3DQEBCwUAMBExDzANBgNVBAMMBnRl
-----END CERTIFICATE-----`

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "registry-ca",
			Namespace: "rhdh-operator",
		},
		Data: map[string]string{
			"ca.crt": caCert,
		},
	}

	dhpc := &api.DevHubPluginCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-catalog",
		},
		Spec: api.DevHubPluginCatalogSpec{
			Source: api.CatalogSource{
				Ref: "oci://registry.example.com/rhdh/plugin-catalog:v1",
				CertificateAuthority: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "registry-ca",
					},
					// Key is empty - should default to "ca.crt"
				},
			},
		},
	}

	r := setupCatalogTestReconciler(dhpc, cm)

	inputs, err := listAndBuildInputs(r, context.TODO())
	require.NoError(t, err)
	require.Len(t, inputs, 1)
	assert.NotNil(t, inputs[0].CACert)
}

func TestBuildCatalogInputs_MultipleCatalogs(t *testing.T) {
	dhpc1 := &api.DevHubPluginCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: "catalog-1",
		},
		Spec: api.DevHubPluginCatalogSpec{
			Source: api.CatalogSource{
				Ref: "oci://registry.example.com/rhdh/plugin-catalog-1:v1",
			},
		},
	}

	dhpc2 := &api.DevHubPluginCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: "catalog-2",
		},
		Spec: api.DevHubPluginCatalogSpec{
			Source: api.CatalogSource{
				Ref: "oci://registry.example.com/rhdh/plugin-catalog-2:v1",
			},
		},
	}

	r := setupCatalogTestReconciler(dhpc1, dhpc2)

	inputs, err := listAndBuildInputs(r, context.TODO())
	require.NoError(t, err)
	assert.Len(t, inputs, 2)
}

func TestBuildCatalogInputs_MissingSecret_Fails(t *testing.T) {
	dhpc := &api.DevHubPluginCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-catalog",
		},
		Spec: api.DevHubPluginCatalogSpec{
			Source: api.CatalogSource{
				Ref: "oci://registry.example.com/rhdh/plugin-catalog:v1",
				PullSecret: &corev1.LocalObjectReference{
					Name: "non-existent-secret",
				},
			},
		},
	}

	r := setupCatalogTestReconciler(dhpc)

	_, err := listAndBuildInputs(r, context.TODO())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get pull secret")
	assert.Contains(t, err.Error(), "non-existent-secret")
}

func TestBuildCatalogInputs_MissingCACert_Fails(t *testing.T) {
	dhpc := &api.DevHubPluginCatalog{
		ObjectMeta: metav1.ObjectMeta{
			Name: "test-catalog",
		},
		Spec: api.DevHubPluginCatalogSpec{
			Source: api.CatalogSource{
				Ref: "oci://registry.example.com/rhdh/plugin-catalog:v1",
				CertificateAuthority: &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: "non-existent-ca",
					},
				},
			},
		},
	}

	r := setupCatalogTestReconciler(dhpc)

	_, err := listAndBuildInputs(r, context.TODO())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "failed to get CA ConfigMap")
	assert.Contains(t, err.Error(), "non-existent-ca")
}

func TestApplyConfigMap_WithExistingDynamicPlugins(t *testing.T) {
	// Simulate default-config ConfigMap with existing dynamic-plugins.yaml
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultConfigMapName,
			Namespace: "rhdh-operator",
		},
		Data: map[string]string{
			"app-config.yaml": "app:\n  title: My Backstage\n",
			"dynamic-plugins.yaml": `plugins:
  - package: "oci://old-registry.com/old-plugin:1.0"
    disabled: false
`,
		},
	}

	r := setupCatalogTestReconciler(existingCM)

	// Patch with new plugins from catalog
	newPluginsYAML := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-techdocs:1.0"
    disabled: false
  - package: "oci://registry.example.com/rhdh/plugin-kubernetes:2.0"
    disabled: false
`
	patchBytes := []byte(`{"data":{"dynamic-plugins.yaml":"` + escapeJSONString(newPluginsYAML) + `",".catalogs-ready":"true"}}`)

	err := r.applyConfigMap(context.TODO(), patchBytes)
	require.NoError(t, err)

	// Verify the ConfigMap was patched
	cm := &corev1.ConfigMap{}
	err = r.Get(context.TODO(), types.NamespacedName{
		Name:      DefaultConfigMapName,
		Namespace: "rhdh-operator",
	}, cm)
	require.NoError(t, err)

	// Verify dynamic-plugins.yaml was replaced with new content
	assert.Contains(t, cm.Data, "dynamic-plugins.yaml")
	dpContent := cm.Data["dynamic-plugins.yaml"]

	// Content must be parseable as DynaPluginsConfig
	var config model.DynaPluginsConfig
	err = yaml.Unmarshal([]byte(dpContent), &config)
	require.NoError(t, err, "dynamic-plugins.yaml should be parseable as DynaPluginsConfig")

	// Verify plugins from catalog are present
	assert.Len(t, config.Plugins, 2)
	assert.Equal(t, "oci://registry.example.com/rhdh/plugin-techdocs:1.0", config.Plugins[0].Package)
	assert.Equal(t, "oci://registry.example.com/rhdh/plugin-kubernetes:2.0", config.Plugins[1].Package)

	// Verify old plugin is NOT present (was replaced)
	for _, p := range config.Plugins {
		assert.NotContains(t, p.Package, "old-registry.com")
	}

	// Verify catalogs-ready marker
	assert.Equal(t, "true", cm.Data[".catalogs-ready"])

	// Verify other keys are preserved
	assert.Equal(t, "app:\n  title: My Backstage\n", cm.Data["app-config.yaml"])
}

func TestApplyConfigMap_WithoutExistingDynamicPlugins(t *testing.T) {
	// Simulate default-config ConfigMap WITHOUT dynamic-plugins.yaml
	existingCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DefaultConfigMapName,
			Namespace: "rhdh-operator",
		},
		Data: map[string]string{
			"app-config.yaml": "app:\n  title: My Backstage\n",
		},
	}

	r := setupCatalogTestReconciler(existingCM)

	// Patch with plugins from catalog
	newPluginsYAML := `plugins:
  - package: "oci://registry.example.com/rhdh/plugin-argocd:1.0"
    disabled: false
`
	patchBytes := []byte(`{"data":{"dynamic-plugins.yaml":"` + escapeJSONString(newPluginsYAML) + `",".catalogs-ready":"true"}}`)

	err := r.applyConfigMap(context.TODO(), patchBytes)
	require.NoError(t, err)

	// Verify the ConfigMap was patched
	cm := &corev1.ConfigMap{}
	err = r.Get(context.TODO(), types.NamespacedName{
		Name:      DefaultConfigMapName,
		Namespace: "rhdh-operator",
	}, cm)
	require.NoError(t, err)

	// Verify dynamic-plugins.yaml was created
	assert.Contains(t, cm.Data, "dynamic-plugins.yaml")
	dpContent := cm.Data["dynamic-plugins.yaml"]

	// Content must be parseable as DynaPluginsConfig
	var config model.DynaPluginsConfig
	err = yaml.Unmarshal([]byte(dpContent), &config)
	require.NoError(t, err, "dynamic-plugins.yaml should be parseable as DynaPluginsConfig")

	// Verify plugin from catalog is present
	assert.Len(t, config.Plugins, 1)
	assert.Equal(t, "oci://registry.example.com/rhdh/plugin-argocd:1.0", config.Plugins[0].Package)

	// Verify catalogs-ready marker
	assert.Equal(t, "true", cm.Data[".catalogs-ready"])

	// Verify other keys are preserved
	assert.Equal(t, "app:\n  title: My Backstage\n", cm.Data["app-config.yaml"])
}

// escapeJSONString escapes a string for inclusion in a JSON string value
func escapeJSONString(s string) string {
	// Replace newlines and quotes for JSON embedding
	result := ""
	for _, c := range s {
		switch c {
		case '\n':
			result += "\\n"
		case '"':
			result += "\\\""
		case '\\':
			result += "\\\\"
		default:
			result += string(c)
		}
	}
	return result
}

func TestConstants(t *testing.T) {
	assert.Equal(t, "rhdh-default-config", DefaultConfigMapName)
}
