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

	"github.com/redhat-developer/rhdh-operator/api"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

const (
	SecretObjectKind    = "Secret"
	ConfigMapObjectKind = "ConfigMap"
)

const BackstageImageEnvVar = "RELATED_IMAGE_backstage"
const DefaultMountDir = "/opt/app-root/src"
const ExtConfigHashAnnotation = "rhdh.redhat.com/ext-config-hash"
const ListMergeAnnotation = "rhdh.redhat.com/deployment-patch-list-merge-mode"

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
	registerConfig(DeploymentKey, BackstageDeploymentFactory{}, false, mergeDeployments)
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
func (b *BackstageDeployment) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	// Set the deployment from config parameter if not nil
	if config != nil {
		var err error
		b.deployable, err = CreateDeployable(config)
		if err != nil {
			return fmt.Errorf("cannot set deployment object: %w", err)
		}
	}

	b.model = model

	// Call setMetaInfo if deployment is not nil
	if b.deployable.GetObject() != nil {
		model.setRuntimeObject(DeploymentKey, b)
		b.setMetaInfo(backstage, scheme)

		if BackstageContainerIndex(b.podSpec()) < 0 {
			return fmt.Errorf("backstage Deployment is not initialized, Backstage Container is not identified")
		}

		if b.deployable.PodObjectMeta().Annotations == nil {
			b.deployable.PodObjectMeta().Annotations = map[string]string{}
		}
		b.deployable.PodObjectMeta().Annotations[ExtConfigHashAnnotation] = model.ExternalConfig.WatchingHash

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
			return err
		}
	if err := b.setDeployment(backstage); err != nil {
		return false, err
	}

	return nil
}

// implementation of RuntimeObject interface
func (b *BackstageDeployment) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {

	//DbSecret
	var err error
	if backstage.Spec.IsAuthSecretSpecified() {
		err = b.addEnvVarsFrom(containersFilter{}, SecretObjectKind, backstage.Spec.Database.AuthSecretName, "")
	} else if dbSecret := b.model.GetRuntimeObject(DbSecretKey); dbSecret != nil {
		secret := dbSecret.(*DbSecret).secret
		if secret != nil {
			err = b.addEnvVarsFrom(containersFilter{}, SecretObjectKind, secret.Name, "")
		}
	}

	if err != nil {
		return fmt.Errorf("can not add env vars from db secret: %w", err)
	}

	return nil
}

func (b *BackstageDeployment) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
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
func (b *BackstageDeployment) setDeployment(backstage api.Backstage) error {

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

			mergeOpts := kyaml.MergeOptions{}
			switch backstage.GetAnnotations()[ListMergeAnnotation] {
			case "prepend":
				mergeOpts.ListIncreaseDirection = kyaml.MergeOptionsListPrepend
			case "append":
				mergeOpts.ListIncreaseDirection = kyaml.MergeOptionsListAppend
			}

			merged, err := merge2.MergeStrings(string(conf.Raw), string(deplStr), false, mergeOpts)
			if err != nil {
				return fmt.Errorf("can not merge spec.deployment: %w", err)
			}

			// TODO(asoro): once https://github.com/kubernetes-sigs/kustomize/issues/6146 is resolved,
			// remove this second pass and use only ListPrepend above.
			if mergeOpts.ListIncreaseDirection == kyaml.MergeOptionsListPrepend {
				merged, err = merge2.MergeStrings(string(conf.Raw), merged, false, kyaml.MergeOptions{})
				if err != nil {
					return fmt.Errorf("can not merge spec.deployment: %w", err)
				}
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
func (b *BackstageDeployment) getDefConfigMountPath(obj client.Object) (mountPath, subPath, fileName string) {

	// mountPath, no subPath or subPath="" - mount folder
	// mountPath, subPath="*" (must not work for Secrets?) or "list,of,keys" - mount files one-by-one to mountPath
	// no mountPath (subPath does not matter) - mount files to defaultMountPath()

	mountPath = obj.GetAnnotations()[DefaultMountPathAnnotation]
	if mountPath == "" {
		mountPath = b.defaultMountPath()
	}
	subPath = obj.GetAnnotations()[DefaultSubPathAnnotation]
	fileName = ""
	if subPath != "" && subPath != "*" {
		fileName = subPath
	}
	//if mountPath == "" {
	//	return
	//}
	//return mountPath, subPath, fileName
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
func (b *BackstageDeployment) addExtraEnvs(extraEnvs *api.ExtraEnvs) error {
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

// mergeDeployments merges deployment configurations from base and flavours
// Uses merge2.MergeStrings to merge YAML, then applies platform patch
func mergeDeployments(sources []configSource, scheme runtime.Scheme, platformExt string) ([]client.Object, error) {
	if len(sources) == 0 {
		return []client.Object{}, nil
	}
	mergedYAML := sources[0].content

	// Merge with flavour patches
	for i := 1; i < len(sources); i++ {
		mergedStr, err := merge2.MergeStrings(string(sources[i].content), string(mergedYAML), false, kyaml.MergeOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to merge deployment from %s: %w", sources[i].path, err)
		}
		mergedYAML = []byte(mergedStr)
	}

	// Apply platform patch to the merged result
	platformPatch, err := utils.ReadPlatformPatch(sources[0].path, platformExt)
	if err != nil {
		return nil, fmt.Errorf("failed to read platform patch: %w", err)
	}

	// Parse with platform patch applied
	objs, err := utils.ReadYamls(mergedYAML, platformPatch, scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to parse merged deployment: %w", err)
	}

	if len(objs) == 0 {
		paths := make([]string, len(sources))
		for i, s := range sources {
			paths[i] = s.path
		}
		return nil, fmt.Errorf("no objects found after merging deployment sources: %v", paths)
	}
	return []client.Object{objs[0]}, nil
}
