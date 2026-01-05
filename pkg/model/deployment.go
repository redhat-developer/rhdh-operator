package model

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/kustomize/kyaml/yaml/merge2"

	"sigs.k8s.io/yaml"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha5"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

const (
	SecretObjectKind    = "Secret"
	ConfigMapObjectKind = "ConfigMap"
)

const BackstageImageEnvVar = "RELATED_IMAGE_backstage"
const CatalogIndexImageEnvVar = "RELATED_IMAGE_catalog_index"
const DefaultMountDir = "/opt/app-root/src"
const ExtConfigHashAnnotation = "rhdh.redhat.com/ext-config-hash"

type BackstageDeploymentFactory struct{}

type ObjectKind string

func (f BackstageDeploymentFactory) newBackstageObject() RuntimeObject {
	return &BackstageDeployment{}
}

type BackstageDeployment struct {
	deployable Deployable
	model      *BackstageModel
}

func init() {
	registerConfig("deployment.yaml", BackstageDeploymentFactory{}, false)
}

func DeploymentName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage")
}

// BackstageContainerIndex returns the index of backstage container in from deployment.spec.template.spec.containers array
func BackstageContainerIndex(bsdPod *corev1.PodSpec) int {
	for i, c := range bsdPod.Containers {
		if c.Name == BackstageContainerName() {
			return i
		}
	}
	return -1
}

func BackstageContainerName() string {
	return "backstage-backend"
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) Object() runtime.Object {
	return b.deployable.GetObject()
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) setObject(obj runtime.Object) {

	//b.deployable = DeploymentObj{}

	//if obj != nil {
	//	b.deployable.setObject(obj)
	//}
	var err error
	b.deployable, err = CreateDeployable(obj)
	if err != nil {
		panic(fmt.Sprintf("cannot set deployment object: %v", err))
	}
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {
	if b.deployable.GetObject() == nil {
		return false, fmt.Errorf("backstage Deployment is not initialized, make sure there is deployment.yaml in default or raw configuration")
	}

	if BackstageContainerIndex(b.podSpec()) < 0 {
		return false, fmt.Errorf("backstage Deployment is not initialized, Backstage Container is not identified")
	}

	if b.deployable.PodObjectMeta().Annotations == nil {
		b.deployable.PodObjectMeta().Annotations = map[string]string{}
	}
	b.deployable.PodObjectMeta().Annotations[ExtConfigHashAnnotation] = model.ExternalConfig.WatchingHash

	model.backstageDeployment = b
	model.setRuntimeObject(b)
	b.model = model

	// override image with env var
	if os.Getenv(BackstageImageEnvVar) != "" {
		b.setImage(ptr.To(os.Getenv(BackstageImageEnvVar)))
	}

	// Set CATALOG_INDEX_IMAGE from operator env var BEFORE extraEnvs are applied, so user-specified extraEnvs can still override this value
	if catalogIndexImage := os.Getenv(CatalogIndexImageEnvVar); catalogIndexImage != "" {
		if i, _ := DynamicPluginsInitContainer(b.podSpec().InitContainers); i >= 0 {
			b.setOrAppendEnvVar(&b.podSpec().InitContainers[i], "CATALOG_INDEX_IMAGE", catalogIndexImage)
		}
	}

	if err := b.setDeployment(backstage); err != nil {
		return false, err
	}

	return true, nil
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) updateAndValidate(backstage bsv1.Backstage) error {

	//DbSecret
	var err error
	if backstage.Spec.IsAuthSecretSpecified() {
		err = b.addEnvVarsFrom(containersFilter{}, SecretObjectKind, backstage.Spec.Database.AuthSecretName, "")
	} else if b.model.LocalDbSecret != nil {
		err = b.addEnvVarsFrom(containersFilter{}, SecretObjectKind, b.model.LocalDbSecret.secret.Name, "")
	}

	if err != nil {
		return fmt.Errorf("can not add env vars from db secret: %w", err)
	}

	return nil
}

func (b *BackstageDeployment) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.deployable.GetObject().SetName(DeploymentName(backstage.Name))
	utils.GenerateLabel(&b.deployable.PodObjectMeta().Labels, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))

	b.deployable.SpecSelector().MatchLabels[BackstageAppLabel] = utils.BackstageAppLabelValue(backstage.Name)
	setMetaInfo(b.deployable.GetObject(), backstage, scheme)

}

func (b *BackstageDeployment) container() *corev1.Container {
	return &b.podSpec().Containers[BackstageContainerIndex(b.podSpec())]
}

func (b *BackstageDeployment) containerByName(name string) *corev1.Container {
	for i, c := range b.podSpec().Containers {
		if c.Name == name {
			return &b.podSpec().Containers[i]
		}
	}
	for i, c := range b.podSpec().InitContainers {
		if c.Name == name {
			return &b.podSpec().InitContainers[i]
		}
	}
	return nil
}

func (b *BackstageDeployment) allContainers() []string {
	containers := []string{}
	spec := b.podSpec()
	for _, c := range spec.Containers {
		containers = append(containers, c.Name)
	}
	for _, c := range spec.InitContainers {
		containers = append(containers, c.Name)
	}
	return containers
}

func (b *BackstageDeployment) podSpec() *corev1.PodSpec {
	return b.deployable.PodSpec()
}

func (b *BackstageDeployment) defaultMountPath() string {
	dmp := b.container().WorkingDir
	if dmp == "" {
		return DefaultMountDir
	}
	return dmp
}

func (b *BackstageDeployment) mountPath(objectMountPath, objectKey, sharedMountPath string) (string, bool) {

	mp := b.defaultMountPath()
	if sharedMountPath != "" {
		mp = sharedMountPath
	}

	wSubpath := true
	if objectMountPath != "" {
		if filepath.IsAbs(objectMountPath) {
			mp = objectMountPath
		} else {
			mp = filepath.Join(mp, objectMountPath)
		}

		if objectKey == "" {
			wSubpath = false
		}
	}

	return mp, wSubpath
}

// setDeployment sets the deployment object from the backstage configuration
// it merges the deployment object with the patch from the backstage configuration
func (b *BackstageDeployment) setDeployment(backstage bsv1.Backstage) error {

	// set from backstage.Spec.Deployment
	if backstage.Spec.Deployment != nil {

		if backstage.Spec.Deployment.Kind != "" {
			dw, err := b.deployable.ConvertTo(backstage.Spec.Deployment.Kind)
			if err != nil {
				return fmt.Errorf("can not convert deployment to kind %s: %w", backstage.Spec.Deployment.Kind, err)
			}
			b.deployable = dw
		}

		if conf := backstage.Spec.Deployment.Patch; conf != nil {

			deplStr, err := yaml.Marshal(b.deployable.GetObject())
			if err != nil {
				return fmt.Errorf("can not marshal deployment object: %w", err)
			}

			merged, err := merge2.MergeStrings(string(conf.Raw), string(deplStr), false, kyaml.MergeOptions{})
			if err != nil {
				return fmt.Errorf("can not merge spec.deployment: %w", err)
			}

			b.deployable.SetEmpty()
			err = yaml.Unmarshal([]byte(merged), b.deployable.GetObject())
			if err != nil {
				return fmt.Errorf("can not unmarshal merged deployment: %w", err)
			}
		}
	}

	// call it after setting from backstage.Spec.Deployment
	if backstage.Spec.Application != nil {
		if err := b.addExtraEnvs(backstage.Spec.Application.ExtraEnvs); err != nil {
			return fmt.Errorf("can not add extra envs: %w", err)
		}
	}
	return nil
}

// getDefConfigMountPath returns the mount path and subpath (defined in default configuration)
func (b *BackstageDeployment) getDefConfigMountPath(obj client.Object) (mountPath string, subPath string) {

	mountPath, ok := obj.GetAnnotations()[DefaultMountPathAnnotation]
	subPath = ""
	if !ok {
		volName := utils.ToRFC1123Label(obj.GetName())
		mountPath = filepath.Join(b.defaultMountPath(), volName)
		subPath = volName
	}
	return
}

// sets container image name of Backstage Container
func (b *BackstageDeployment) setImage(image *string) {
	if image != nil {
		b.container().Image = *image
		// this is a workaround for RHDH configuration
		// it is not a fact that all the containers should be updated
		// in general case need something smarter
		// to mark/recognize containers for update
		if len(b.podSpec().InitContainers) > 0 {
			i, ic := DynamicPluginsInitContainer(b.podSpec().InitContainers)
			if ic != nil {
				b.podSpec().InitContainers[i].Image = *image
			}
		}
	}
}

// adds environment from source to the Backstage Container
// If an env var with the same name already exists, it will be replaced (not duplicated)
func (b *BackstageDeployment) addExtraEnvs(extraEnvs *bsv1.ExtraEnvs) error {
	if extraEnvs == nil {
		return nil
	}
	for _, env := range extraEnvs.Envs {
		filter := containersFilter{names: env.Containers}
		containers, err := filter.getContainers(b)
		if err != nil {
			return fmt.Errorf("can not get containers to add env %s: %w", env.Name, err)
		}
		for _, container := range containers {
			b.setOrAppendEnvVar(container, env.Name, env.Value)
		}
	}
	return nil
}

// setOrAppendEnvVar sets an env var on a container, replacing if it exists or appending if not
func (b *BackstageDeployment) setOrAppendEnvVar(container *corev1.Container, name, value string) {
	for i, existingEnv := range container.Env {
		if existingEnv.Name == name {
			container.Env[i] = corev1.EnvVar{Name: name, Value: value}
			return
		}
	}
	container.Env = append(container.Env, corev1.EnvVar{Name: name, Value: value})
}

// MountFilesFrom adds Volume to specified podSpec and related VolumeMounts to specified belonging to this podSpec container
// from ConfigMap or Secret volume source
// containers - array of containers to add VolumeMount(s) to
// kind - kind of source, can be ConfigMap or Secret
// objectName - name of source object
// mountPath - mount path, default one or  as it specified in BackstageCR.spec.Application.AppConfig|ExtraFiles
// fileName - file name which fits one of the object's key, otherwise error will be returned.
// withSubPath - if true will be mounted file-by-file with subpath, otherwise will be mounted as directory to specified path
// dataKeys - keys for ConfigMap/Secret data
func (b *BackstageDeployment) mountFilesFrom(containersFilter containersFilter, kind ObjectKind, objectName, mountPath, fileName string, withSubPath bool, dataKeys []string) error {

	containers, err := containersFilter.getContainers(b)
	if err != nil {
		return fmt.Errorf("can not get containers to mount %s: %w", objectName, err)
	}

	volName := utils.GenerateVolumeNameFromCmOrSecret(objectName)
	volSrc := corev1.VolumeSource{}

	switch kind {
	case ConfigMapObjectKind:
		volSrc.ConfigMap = &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
			DefaultMode:          ptr.To(int32(420)),
			Optional:             ptr.To(false),
		}
	case SecretObjectKind:
		volSrc.Secret = &corev1.SecretVolumeSource{
			SecretName:  objectName,
			DefaultMode: ptr.To(int32(420)),
			Optional:    ptr.To(false),
		}
	}

	b.podSpec().Volumes = append(b.podSpec().Volumes, corev1.Volume{Name: volName, VolumeSource: volSrc})

	for _, container := range containers {
		var newMounts []corev1.VolumeMount
		replaced := false

		// Prepare the new VolumeMount(s)
		var mountsToAdd []corev1.VolumeMount
		if !withSubPath {
			mountsToAdd = []corev1.VolumeMount{{Name: volName, MountPath: mountPath}}
		} else if len(dataKeys) > 0 {
			for _, file := range dataKeys {
				if fileName == "" || fileName == file {
					mountsToAdd = append(mountsToAdd, corev1.VolumeMount{
						Name: volName, MountPath: filepath.Join(mountPath, file), SubPath: file, ReadOnly: true,
					})
				}
			}
		} else {
			mountsToAdd = []corev1.VolumeMount{{Name: volName, MountPath: filepath.Join(mountPath, fileName), SubPath: fileName, ReadOnly: true}}
		}

		// Replace or append
		for _, mount := range container.VolumeMounts {
			replacedHere := false
			for _, newMount := range mountsToAdd {
				if mount.MountPath == newMount.MountPath {
					newMounts = append(newMounts, newMount)
					replaced = true
					replacedHere = true
					break
				}
			}
			if !replacedHere {
				newMounts = append(newMounts, mount)
			}
		}
		if !replaced {
			newMounts = append(newMounts, mountsToAdd...)
		}
		container.VolumeMounts = newMounts
	}

	return nil
}

// AddEnvVarsFrom adds environment variable to specified containers
// containersFilter - filter to get containers to add env variable to
// kind - kind of source, can be ConfigMap or Secret
// objectName - name of source object
// varName - name of env variable

func (b *BackstageDeployment) addEnvVarsFrom(containersFilter containersFilter, kind ObjectKind, objectName, varName string) error {

	containers, err := containersFilter.getContainers(b)
	if err != nil {
		return fmt.Errorf("can not get containers to add env %s: %w", varName, err)
	}

	for _, container := range containers {
		if varName == "" {
			envFromSrc := corev1.EnvFromSource{}
			switch kind {
			case ConfigMapObjectKind:
				envFromSrc.ConfigMapRef = &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
				}
			case SecretObjectKind:
				envFromSrc.SecretRef = &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
				}
			default:
				// unknown kind
				return fmt.Errorf("unknown object kind %s to add env vars from", kind)
			}
			container.EnvFrom = append(container.EnvFrom, envFromSrc)
		} else {
			envVarSrc := &corev1.EnvVarSource{}
			switch kind {
			case ConfigMapObjectKind:
				envVarSrc.ConfigMapKeyRef = &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
					Key:                  varName,
				}
			case SecretObjectKind:
				envVarSrc.SecretKeyRef = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
					Key:                  varName,
				}
			default:
				// unknown kind
				return fmt.Errorf("unknown object kind %s to add env vars from", kind)
			}
			container.Env = append(container.Env, corev1.EnvVar{
				Name:      varName,
				ValueFrom: envVarSrc,
			})
		}
	}
	return nil
}
