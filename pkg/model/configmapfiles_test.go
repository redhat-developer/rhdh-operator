package model

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/platform"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/stretchr/testify/assert"
)

var (
	configMapFilesTestBackstage = api.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs",
			Namespace: "ns123",
		},
		Spec: api.BackstageSpec{
			Application: &api.Application{
				ExtraFiles: &api.ExtraFiles{
					MountPath:  "/my/path",
					ConfigMaps: []api.FileObjectRef{},
				},
			},
		},
	}
)

func TestDefaultConfigMapFiles(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-files.yaml", "raw-cm-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.container().VolumeMounts))
	assert.Equal(t, 1, len(deployment.podSpec().Volumes))

}

func TestSpecifiedConfigMapFiles(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()
	cmf := &bs.Spec.Application.ExtraFiles.ConfigMaps
	*cmf = append(*cmf, api.FileObjectRef{Name: "cm1"})
	*cmf = append(*cmf, api.FileObjectRef{Name: "cm2", MountPath: "/custom/path"})
	*cmf = append(*cmf, api.FileObjectRef{Name: "cm3", MountPath: "rel"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.ExtraFileConfigMapKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraFileConfigMapKeys["cm1"] = NewDataObjectKeys(map[string]string{"conf1.yaml": "data"}, nil)
	testObj.externalConfig.ExtraFileConfigMapKeys["cm2"] = NewDataObjectKeys(map[string]string{"conf2.yaml": "data"}, nil)
	testObj.externalConfig.ExtraFileConfigMapKeys["cm3"] = NewDataObjectKeys(map[string]string{"conf3.yaml": "data"}, nil)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 3, len(deployment.container().VolumeMounts))
	assert.Equal(t, 0, len(deployment.container().Args))
	assert.Equal(t, 3, len(deployment.deployable.PodSpec().Volumes))

	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("cm1"), deployment.container().VolumeMounts[0].Name)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("cm2"), deployment.container().VolumeMounts[1].Name)

	assert.Equal(t, "/my/path/conf1.yaml", deployment.container().VolumeMounts[0].MountPath)
	assert.Equal(t, "/custom/path", deployment.container().VolumeMounts[1].MountPath)
	assert.Equal(t, "/my/path/rel", deployment.container().VolumeMounts[2].MountPath)

	assert.Equal(t, "conf1.yaml", deployment.container().VolumeMounts[0].SubPath)
	assert.Equal(t, "", deployment.container().VolumeMounts[1].SubPath)
	assert.Equal(t, "", deployment.container().VolumeMounts[2].SubPath)

}

func TestDefaultAndSpecifiedConfigMapFiles(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()
	cmf := &bs.Spec.Application.ExtraFiles.ConfigMaps
	*cmf = append(*cmf, api.FileObjectRef{Name: appConfigTestCm.Name})

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-files.yaml", "raw-cm-files.yaml")

	testObj.externalConfig.ExtraFileConfigMapKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraFileConfigMapKeys[appConfigTestCm.Name] = NewDataObjectKeys(nil, map[string][]byte{"conf1.yaml": []byte("data")})

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 2, len(deployment.container().VolumeMounts))
	assert.Equal(t, 0, len(deployment.container().Args))
	assert.Equal(t, 2, len(deployment.podSpec().Volumes))

}

func TestConfigMapFilesMountPathReplacement(t *testing.T) {
	// Test that CR spec ConfigMap replaces default config ConfigMap when mounting to the same path

	bs := *configMapFilesTestBackstage.DeepCopy()

	// CR spec ConfigMap mounting as directory (no Key specified) to the same custom path
	cmf := &bs.Spec.Application.ExtraFiles.ConfigMaps
	*cmf = append(*cmf, api.FileObjectRef{
		Name:      "cr-configmap",
		MountPath: "/custom/mount", // Same path as default will use
	})

	// Add default config with a ConfigMap that also mounts to /custom/mount
	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-files.yaml", "raw-cm-files-custom-path.yaml")

	// CR ConfigMap - mounts to same path, should replace default
	testObj.externalConfig.ExtraFileConfigMapKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraFileConfigMapKeys["cr-configmap"] = NewDataObjectKeys(map[string]string{
		"file1.yaml": "CR data 1",
		"file2.yaml": "CR data 2",
	}, nil)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	// Should have only 1 volume mount (CR replaced default at same path)
	assert.Equal(t, 1, len(deployment.container().VolumeMounts))

	// Both volumes should be defined in pod spec (even though one isn't mounted)
	assert.Equal(t, 2, len(deployment.podSpec().Volumes))

	// Verify the mount is to /custom/mount
	mount := deployment.container().VolumeMounts[0]
	assert.Equal(t, "/custom/mount", mount.MountPath)

	// Verify it's the CR ConfigMap that's mounted (volume name should be from cr-configmap)
	assert.Equal(t, utils.GenerateVolumeNameFromCmOrSecret("cr-configmap"), mount.Name)

	// No subPath since mounting entire directory
	assert.Equal(t, "", mount.SubPath)
}

func TestSpecifiedConfigMapFilesWithBinaryData(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()
	cmf := &bs.Spec.Application.ExtraFiles.ConfigMaps
	*cmf = append(*cmf, api.FileObjectRef{Name: "cm1"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.ExtraFileConfigMapKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraFileConfigMapKeys["cm1"] = NewDataObjectKeys(nil, map[string][]byte{"conf1.yaml": []byte("data")})

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 1, len(deployment.container().VolumeMounts))
	// file name (cm.Data.key) is expected to be a part of mountPath
	assert.Equal(t, filepath.Join("/my/path", "conf1.yaml"), deployment.container().VolumeMounts[0].MountPath)

}

func TestSpecifiedCMFilesWithContainers(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()
	cmf := &bs.Spec.Application.ExtraFiles.ConfigMaps
	*cmf = append(*cmf, api.FileObjectRef{Name: "cm1", Containers: []string{"install-dynamic-plugins", "another-container"}})
	*cmf = append(*cmf, api.FileObjectRef{Name: "cm2", MountPath: "/custom/path", Containers: []string{"install-dynamic-plugins"}})
	*cmf = append(*cmf, api.FileObjectRef{Name: "cm3", MountPath: "rel", Containers: []string{"*"}})

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("deployment.yaml", "multicontainer-deployment.yaml")

	testObj.externalConfig.ExtraFileConfigMapKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraFileConfigMapKeys["cm1"] = NewDataObjectKeys(map[string]string{"conf1.yaml": "data"}, nil)
	testObj.externalConfig.ExtraFileConfigMapKeys["cm2"] = NewDataObjectKeys(map[string]string{"conf2.yaml": "data"}, nil)
	testObj.externalConfig.ExtraFileConfigMapKeys["cm3"] = NewDataObjectKeys(map[string]string{"conf3.yaml": "data"}, nil)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.True(t, len(model.RuntimeObjects) > 0)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	assert.Equal(t, 3, len(deployment.containerByName("install-dynamic-plugins").VolumeMounts))
	assert.Equal(t, 1, len(deployment.containerByName("backstage-backend").VolumeMounts))
	assert.Equal(t, "cm3", deployment.containerByName("backstage-backend").VolumeMounts[0].Name)
	assert.Equal(t, 2, len(deployment.containerByName("another-container").VolumeMounts))

}

func TestCMFilesWithNonExistedContainerFailed(t *testing.T) {
	bs := *configMapFilesTestBackstage.DeepCopy()
	bs.Spec.Application = &api.Application{
		ExtraFiles: &api.ExtraFiles{
			ConfigMaps: []api.FileObjectRef{
				{
					Name:       "cmName",
					Containers: []string{"another-container"},
				},
			},
		},
	}

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	_, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.ErrorContains(t, err, "not found")

}

func TestDefaultSubpath(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-files.yaml", "raw-cm-subpath.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	mts := deployment.container().VolumeMounts
	assert.Equal(t, 5, len(mts))

	data1 := findVolumeMountByPath(mts, DefaultMountDir+"/data1")
	assert.NotEmpty(t, data1)
	assert.Equal(t, "data1", data1.SubPath)

	data3 := findVolumeMountByPath(mts, "/mount/path2/data3")
	assert.NotEmpty(t, data3)
	assert.Equal(t, "data3", data3.SubPath)

	data5 := findVolumeMountByPath(mts, "/mount/path3/data5")
	assert.NotEmpty(t, data5)
	assert.Equal(t, "data5", data5.SubPath)

	assert.Empty(t, findVolumeMountByPath(mts, "/mount/path3/data6"))

}

func TestMultiObjectConfigMapFilesInDefaultConfig(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()

	testObj := createBackstageTest(bs).withDefaultConfig(true).addToDefaultConfig("configmap-files.yaml", "multi-cm-files.yaml")

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	// Should have 2 volume mounts for the 2 ConfigMaps
	assert.Equal(t, 2, len(deployment.container().VolumeMounts))
	assert.Equal(t, 2, len(deployment.podSpec().Volumes))

	// Verify volume names
	volumeNames := make(map[string]bool)
	for _, volume := range deployment.podSpec().Volumes {
		volumeNames[volume.Name] = true
	}

	assert.True(t, volumeNames["backstage-files-bs-cm-files-1"])
	assert.True(t, volumeNames["backstage-files-bs-cm-files-2"])
}

func TestMultiObjectConfigMapFilesInSpec(t *testing.T) {

	bs := *configMapFilesTestBackstage.DeepCopy()
	cmf := &bs.Spec.Application.ExtraFiles.ConfigMaps
	*cmf = append(*cmf, api.FileObjectRef{Name: "cm1"})
	*cmf = append(*cmf, api.FileObjectRef{Name: "cm2"})

	testObj := createBackstageTest(bs).withDefaultConfig(true)

	testObj.externalConfig.ExtraFileConfigMapKeys = map[string]DataObjectKeys{}
	testObj.externalConfig.ExtraFileConfigMapKeys["cm1"] = NewDataObjectKeys(map[string]string{"file1.yaml": "data"}, nil)
	testObj.externalConfig.ExtraFileConfigMapKeys["cm2"] = NewDataObjectKeys(map[string]string{"file2.yaml": "data"}, nil)

	model, err := InitObjects(context.TODO(), bs, testObj.externalConfig, platform.Default, testObj.scheme)

	assert.NoError(t, err)
	assert.NotNil(t, model)

	deployment := model.backstageDeployment
	assert.NotNil(t, deployment)

	// Should have 2 volume mounts for the 2 ConfigMaps
	assert.Equal(t, 2, len(deployment.container().VolumeMounts))
	assert.Equal(t, 2, len(deployment.podSpec().Volumes))

	// Verify volume names
	volumeNames := make(map[string]bool)
	for _, volume := range deployment.podSpec().Volumes {
		volumeNames[volume.Name] = true
	}

	assert.True(t, volumeNames[utils.GenerateVolumeNameFromCmOrSecret("cm1")])
	assert.True(t, volumeNames[utils.GenerateVolumeNameFromCmOrSecret("cm2")])
}
