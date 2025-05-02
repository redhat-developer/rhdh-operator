package model

import (
	"fmt"
	"os"
	"path/filepath"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/utils/ptr"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"
	"sigs.k8s.io/kustomize/kyaml/yaml/merge2"
	"sigs.k8s.io/yaml"

	corev1 "k8s.io/api/core/v1"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
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
	__sealights__.TraceFunc("a0856a433efb226c4b")
	return &BackstageDeployment{}
}

type BackstageDeployment struct {
	deployment *appsv1.Deployment
}

func init() {
	__sealights__.TraceFunc("12a76b3e1cbbf1a34b")
	registerConfig("deployment.yaml", BackstageDeploymentFactory{}, false)
}

func DeploymentName(backstageName string) string {
	__sealights__.TraceFunc("1ca02d6160d6d0f107")
	return utils.GenerateRuntimeObjectName(backstageName, "backstage")
}

// BackstageContainerIndex returns the index of backstage container in from deployment.spec.template.spec.containers array
func BackstageContainerIndex(bsd *appsv1.Deployment) int {
	__sealights__.TraceFunc("3111e2599480ea0e20")
	for i, c := range bsd.Spec.Template.Spec.Containers {
		if c.Name == BackstageContainerName() {
			return i
		}
	}
	return -1
}

func BackstageContainerName() string {
	__sealights__.TraceFunc("8492366e431db55470")
	return "backstage-backend"
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) Object() runtime.Object {
	__sealights__.TraceFunc("b5d2af27ac78a66570")
	return b.deployment
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) setObject(obj runtime.Object) {
	__sealights__.TraceFunc("8c0d04da9a1e0c21e9")
	b.deployment = nil
	if obj != nil {
		b.deployment = obj.(*appsv1.Deployment)
	}
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) EmptyObject() client.Object {
	__sealights__.TraceFunc("665af65884c20ab4a7")
	return &appsv1.Deployment{}
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("da4fe6be077311b3e4")
	if b.deployment == nil {
		return false, fmt.Errorf("Backstage Deployment is not initialized, make sure there is deployment.yaml in default or raw configuration")
	}

	if BackstageContainerIndex(b.deployment) < 0 {
		return false, fmt.Errorf("Backstage Deployment is not initialized, Backstage Container is not identified")
	}

	if b.deployment.Spec.Template.ObjectMeta.Annotations == nil {
		b.deployment.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	b.deployment.Spec.Template.ObjectMeta.Annotations[ExtConfigHashAnnotation] = model.ExternalConfig.WatchingHash

	model.backstageDeployment = b
	model.setRuntimeObject(b)

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
func (b *BackstageDeployment) updateAndValidate(model *BackstageModel, backstage bsv1.Backstage) error {
	__sealights__.TraceFunc("8ce9e5582f5f462a90")

	//DbSecret
	if backstage.Spec.IsAuthSecretSpecified() {
		b.addEnvVarsFrom([]string{BackstageContainerName()}, SecretObjectKind, backstage.Spec.Database.AuthSecretName, "")
		//utils.SetDbSecretEnvVar(b.container(), backstage.Spec.Database.AuthSecretName)
	} else if model.LocalDbSecret != nil {
		b.addEnvVarsFrom([]string{BackstageContainerName()}, SecretObjectKind, model.LocalDbSecret.secret.Name, "")
		//utils.SetDbSecretEnvVar(b.container(), model.LocalDbSecret.secret.Name)
	}

	return nil
}

func (b *BackstageDeployment) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("ac00d5603ed2cb9666")
	b.deployment.SetName(DeploymentName(backstage.Name))
	utils.GenerateLabel(&b.deployment.Spec.Template.ObjectMeta.Labels, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))
	if b.deployment.Spec.Selector == nil {
		b.deployment.Spec.Selector = &metav1.LabelSelector{}
	}
	utils.GenerateLabel(&b.deployment.Spec.Selector.MatchLabels, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))
	setMetaInfo(b.deployment, backstage, scheme)
}

func (b *BackstageDeployment) container() *corev1.Container {
	__sealights__.TraceFunc("ad24352b11c04da02d")
	return &b.deployment.Spec.Template.Spec.Containers[BackstageContainerIndex(b.deployment)]
}

func (b *BackstageDeployment) containerByName(name string) *corev1.Container {
	__sealights__.TraceFunc("7d27e1597bbb2ef5c2")
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
	__sealights__.TraceFunc("590816e992fa7c207d")
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
	__sealights__.TraceFunc("d2671864db4ac788b5")
	return &b.deployment.Spec.Template.Spec
}

func (b *BackstageDeployment) defaultMountPath() string {
	__sealights__.TraceFunc("c9f6bf2d9050fd30ad")
	dmp := b.container().WorkingDir
	if dmp == "" {
		return DefaultMountDir
	}
	return dmp
}

func (b *BackstageDeployment) mountPath(objectMountPath, objectKey, sharedMountPath string) (string, bool) {
	__sealights__.TraceFunc("9df3cb47a59bf19634")

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
	__sealights__.TraceFunc("e25a73bd2e81cc14fe")

	// set from backstage.Spec.Application
	if backstage.Spec.Application != nil {
		b.setReplicas(backstage.Spec.Application.Replicas)
		utils.SetImagePullSecrets(b.podSpec(), backstage.Spec.Application.ImagePullSecrets)
		b.setImage(backstage.Spec.Application.Image)
		b.addExtraEnvs(backstage.Spec.Application.ExtraEnvs)
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
	return nil
}

// getDefConfigMountInfo returns the mount path, subpath and the containers to mount the object (defined in default configuration) to
func (b *BackstageDeployment) getDefConfigMountInfo(obj client.Object) (mountPath string, subPath string, toContainers []string) {
	__sealights__.TraceFunc("efae2a1fe29d1ff40f")
	mountPath, ok := obj.GetAnnotations()[DefaultMountPathAnnotation]
	subPath = ""
	if !ok {
		volName := utils.ToRFC1123Label(obj.GetName())
		mountPath = filepath.Join(b.defaultMountPath(), volName)
		subPath = volName
	}
	// filter containers to mount the object to
	toContainers = utils.FilterContainers(b.allContainers(), obj.GetAnnotations()[ContainersAnnotation])
	// if no containers specified, mount to Backstage container only
	if toContainers == nil {
		toContainers = []string{BackstageContainerName()}
	}
	return
}

// sets the amount of replicas (used by CR config)
func (b *BackstageDeployment) setReplicas(replicas *int32) {
	__sealights__.TraceFunc("9d67e879e84c659c6d")
	if replicas != nil {
		b.deployment.Spec.Replicas = replicas
	}
}

// sets container image name of Backstage Container
func (b *BackstageDeployment) setImage(image *string) {
	__sealights__.TraceFunc("a765593b37430c21a4")
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

// adds environment variables to the Backstage Container
func (b *BackstageDeployment) addContainerEnvVar(env bsv1.Env) {
	__sealights__.TraceFunc("cae996c7a24a66f3c1")
	b.container().Env =
		append(b.container().Env, corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
}

// adds environment from source to the Backstage Container
func (b *BackstageDeployment) addExtraEnvs(extraEnvs *bsv1.ExtraEnvs) {
	__sealights__.TraceFunc("4ebdfc5059d6d4b48d")
	if extraEnvs != nil {
		for _, e := range extraEnvs.Envs {
			b.addContainerEnvVar(e)
		}
	}
}

// MountFilesFrom adds Volume to specified podSpec and related VolumeMounts to specified belonging to this podSpec container
// from ConfigMap or Secret volume source
// podSpec - PodSpec to add Volume to
// container - container to add VolumeMount(s) to
// kind - kind of source, can be ConfigMap or Secret
// objectName - name of source object
// mountPath - mount path, default one or  as it specified in BackstageCR.spec.Application.AppConfig|ExtraFiles
// fileName - file name which fits one of the object's key, otherwise error will be returned.
// withSubPath - if true will be mounted file-by-file with subpath, otherwise will be mounted as directory to specified path
// dataKeys - keys for ConfigMap/Secret data
func (b *BackstageDeployment) mountFilesFrom(containers []string, kind ObjectKind, objectName, mountPath, fileName string, withSubPath bool, dataKeys []string) {
	__sealights__.TraceFunc("9a54da3a2c4593411d")

	volName := utils.GenerateVolumeNameFromCmOrSecret(objectName)
	volSrc := corev1.VolumeSource{}
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

	for _, c := range containers {
		container := b.containerByName(c)
		if !withSubPath {
			container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{Name: volName, MountPath: mountPath})
			continue
		}

		if len(dataKeys) > 0 {
			for _, file := range dataKeys {
				if fileName == "" || fileName == file {
					vm := corev1.VolumeMount{Name: volName, MountPath: filepath.Join(mountPath, file), SubPath: file, ReadOnly: true}
					container.VolumeMounts = append(container.VolumeMounts, vm)
				}
			}
		} else {
			vm := corev1.VolumeMount{Name: volName, MountPath: filepath.Join(mountPath, fileName), SubPath: fileName, ReadOnly: true}
			container.VolumeMounts = append(container.VolumeMounts, vm)
		}
	}

}

// AddEnvVarsFrom adds environment variable to specified container
// kind - kind of source, can be ConfigMap or Secret
// objectName - name of source object
// varName - name of env variable
func (b *BackstageDeployment) addEnvVarsFrom(containerNames []string, kind ObjectKind, objectName, varName string) {
	__sealights__.TraceFunc("e1397a5e68c9618efc")

	for _, c := range containerNames {
		container := b.containerByName(c)
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
}
