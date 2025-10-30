package model

import (
	"fmt"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/runtime"

	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/kustomize/kyaml/yaml/merge2"

	"sigs.k8s.io/yaml"

	"k8s.io/utils/ptr"

	corev1 "k8s.io/api/core/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
)

const (
	SecretObjectKind    = "Secret"
	ConfigMapObjectKind = "ConfigMap"
)

const BackstageImageEnvVar = "RELATED_IMAGE_backstage"
const DefaultMountDir = "/opt/app-root/src"
const ExtConfigHashAnnotation = "rhdh.redhat.com/ext-config-hash"

type BackstageDeploymentFactory struct{}

type ObjectKind string

func (f BackstageDeploymentFactory) newBackstageObject() RuntimeObject {
	return &BackstageDeployment{}
}

type BackstageDeployment struct {
	deployment *appsv1.Deployment
	model      *BackstageModel
}

func init() {
	registerConfig("deployment.yaml", BackstageDeploymentFactory{}, false)
}

func DeploymentName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, "backstage")
}

// BackstageContainerIndex returns the index of backstage container in from deployment.spec.template.spec.containers array
func BackstageContainerIndex(bsd *appsv1.Deployment) int {
	for i, c := range bsd.Spec.Template.Spec.Containers {
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
	return b.deployment
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) setObject(obj runtime.Object) {
	b.deployment = nil
	if obj != nil {
		b.deployment = obj.(*appsv1.Deployment)
	}
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) EmptyObject() client.Object {
	return &appsv1.Deployment{}
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {
	if b.deployment == nil {
		return false, fmt.Errorf("backstage deployment is not initialized, make sure there is deployment.yaml in default or raw configuration")
	}

	if BackstageContainerIndex(b.deployment) < 0 {
		return false, fmt.Errorf("backstage deployment is not initialized, Backstage Container is not identified")
	}

	if b.deployment.Spec.Template.Annotations == nil {
		b.deployment.Spec.Template.Annotations = map[string]string{}
	}
	b.deployment.Spec.Template.Annotations[ExtConfigHashAnnotation] = model.ExternalConfig.WatchingHash

	model.backstageDeployment = b
	model.setRuntimeObject(b)
	b.model = model

	// override image with env var
	if os.Getenv(BackstageImageEnvVar) != "" {
		b.setImage(ptr.To(os.Getenv(BackstageImageEnvVar)))
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
	b.deployment.SetName(DeploymentName(backstage.Name))
	utils.GenerateLabel(&b.deployment.Spec.Template.Labels, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))
	if b.deployment.Spec.Selector == nil {
		b.deployment.Spec.Selector = &metav1.LabelSelector{}
	}
	utils.GenerateLabel(&b.deployment.Spec.Selector.MatchLabels, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))
	setMetaInfo(b.deployment, backstage, scheme)
}

func (b *BackstageDeployment) container() *corev1.Container {
	return &b.deployment.Spec.Template.Spec.Containers[BackstageContainerIndex(b.deployment)]
}

func (b *BackstageDeployment) containerByName(name string) *corev1.Container {
	for i, c := range b.deployment.Spec.Template.Spec.Containers {
		if c.Name == name {
			return &b.deployment.Spec.Template.Spec.Containers[i]
		}
	}
	for i, c := range b.deployment.Spec.Template.Spec.InitContainers {
		if c.Name == name {
			return &b.deployment.Spec.Template.Spec.InitContainers[i]
		}
	}
	return nil
}

func (b *BackstageDeployment) allContainers() []string {
	containers := []string{}
	spec := b.deployment.Spec.Template.Spec
	for _, c := range spec.Containers {
		containers = append(containers, c.Name)
	}
	for _, c := range spec.InitContainers {
		containers = append(containers, c.Name)
	}
	return containers
}

func (b *BackstageDeployment) podSpec() *corev1.PodSpec {
	return &b.deployment.Spec.Template.Spec
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

	// set from backstage.Spec.Application
	if backstage.Spec.Application != nil {
		b.setReplicas(backstage.Spec.Application.Replicas)
		utils.SetImagePullSecrets(b.podSpec(), backstage.Spec.Application.ImagePullSecrets)
		b.setImage(backstage.Spec.Application.Image)
	}

	// set from backstage.Spec.Deployment
	if backstage.Spec.Deployment != nil {
		if conf := backstage.Spec.Deployment.Patch; conf != nil {

			deplStr, err := yaml.Marshal(b.deployment)
			if err != nil {
				return fmt.Errorf("can not marshal deployment object: %w", err)
			}

			merged, err := merge2.MergeStrings(string(conf.Raw), string(deplStr), false, kyaml.MergeOptions{})
			if err != nil {
				return fmt.Errorf("can not merge spec.deployment: %w", err)
			}

			b.deployment = &appsv1.Deployment{}
			err = yaml.Unmarshal([]byte(merged), b.deployment)
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

// sets the amount of replicas (used by CR config)
func (b *BackstageDeployment) setReplicas(replicas *int32) {
	if replicas != nil {
		b.deployment.Spec.Replicas = replicas
	}
}

// sets container image name of Backstage Container
func (b *BackstageDeployment) setImage(image *string) {
	if image != nil {
		b.container().Image = *image
		// this is a workaround for RHDH/Janus configuration
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
			container.Env =
				append(container.Env, corev1.EnvVar{
					Name:  env.Name,
					Value: env.Value,
				})
		}
	}
	return nil
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
	//nolint:staticcheck // ignore QF1003 for better readability
	if kind == ConfigMapObjectKind {
		volSrc.ConfigMap = &corev1.ConfigMapVolumeSource{
			LocalObjectReference: corev1.LocalObjectReference{Name: objectName},
			DefaultMode:          ptr.To(int32(420)),
			Optional:             ptr.To(false),
		}
	} else if kind == SecretObjectKind {
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

//nolint:staticcheck // ignore QF1003 ln 410 and 420 for better readability
func (b *BackstageDeployment) addEnvVarsFrom(containersFilter containersFilter, kind ObjectKind, objectName, varName string) error {

	containers, err := containersFilter.getContainers(b)
	if err != nil {
		return fmt.Errorf("can not get containers to add env %s: %w", varName, err)
	}

	for _, container := range containers {
		if varName == "" {
			envFromSrc := corev1.EnvFromSource{}
			if kind == ConfigMapObjectKind {
				envFromSrc.ConfigMapRef = &corev1.ConfigMapEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: objectName}}
			} else if kind == SecretObjectKind {
				envFromSrc.SecretRef = &corev1.SecretEnvSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: objectName}}
			}
			container.EnvFrom = append(container.EnvFrom, envFromSrc)
		} else {
			envVarSrc := &corev1.EnvVarSource{}
			if kind == ConfigMapObjectKind {
				envVarSrc.ConfigMapKeyRef = &corev1.ConfigMapKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: objectName,
					},
					Key: varName,
				}
			} else if kind == SecretObjectKind {
				envVarSrc.SecretKeyRef = &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: objectName,
					},
					Key: varName,
				}
			}
			container.Env = append(container.Env, corev1.EnvVar{
				Name:      varName,
				ValueFrom: envVarSrc,
			})
		}
	}
	return nil
}
