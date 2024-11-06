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

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"
)

const BackstageImageEnvVar = "RELATED_IMAGE_backstage"
const DefaultMountDir = "/opt/app-root/src"
const ExtConfigHashAnnotation = "rhdh.redhat.com/ext-config-hash"

type BackstageDeploymentFactory struct{}

func (f BackstageDeploymentFactory) newBackstageObject() RuntimeObject {
	return &BackstageDeployment{}
}

type BackstageDeployment struct {
	deployment *appsv1.Deployment
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
		if c.Name == "backstage-backend" {
			return i
		}
	}
	return -1
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
		return false, fmt.Errorf("Backstage Deployment is not initialized, make sure there is deployment.yaml in default or raw configuration")
	}

	if BackstageContainerIndex(b.deployment) < 0 {
		return false, fmt.Errorf("Backstage Deployment is not initialized, Backstage Container is not identified")
	}

	if b.deployment.Spec.Template.ObjectMeta.Annotations == nil {
		b.deployment.Spec.Template.ObjectMeta.Annotations = map[string]string{}
	}
	b.deployment.Spec.Template.ObjectMeta.Annotations[ExtConfigHashAnnotation] = model.ExternalConfig.GetHash()

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

	//DbSecret
	if backstage.Spec.IsAuthSecretSpecified() {
		utils.SetDbSecretEnvVar(b.container(), backstage.Spec.Database.AuthSecretName)
	} else if model.LocalDbSecret != nil {
		utils.SetDbSecretEnvVar(b.container(), model.LocalDbSecret.secret.Name)
	}

	return nil
}

func (b *BackstageDeployment) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	b.deployment.SetName(DeploymentName(backstage.Name))
	utils.GenerateLabel(&b.deployment.Spec.Template.ObjectMeta.Labels, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))
	if b.deployment.Spec.Selector == nil {
		b.deployment.Spec.Selector = &metav1.LabelSelector{}
	}
	utils.GenerateLabel(&b.deployment.Spec.Selector.MatchLabels, BackstageAppLabel, utils.BackstageAppLabelValue(backstage.Name))
	setMetaInfo(b.deployment, backstage, scheme)
}

func (b *BackstageDeployment) container() *corev1.Container {
	return &b.deployment.Spec.Template.Spec.Containers[BackstageContainerIndex(b.deployment)]
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

func (b *BackstageDeployment) mountPath(objectRef bsv1.FileObjectRef, sharedMountPath string) (string, bool) {

	mp := b.defaultMountPath()
	if sharedMountPath != "" {
		mp = sharedMountPath
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

func (b *BackstageDeployment) setDeployment(backstage bsv1.Backstage) error {

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

// adds environment variables to the Backstage Container
func (b *BackstageDeployment) addContainerEnvVar(env bsv1.Env) {
	b.container().Env =
		append(b.container().Env, corev1.EnvVar{
			Name:  env.Name,
			Value: env.Value,
		})
}

// adds environment from source to the Backstage Container
func (b *BackstageDeployment) addExtraEnvs(extraEnvs *bsv1.ExtraEnvs) {
	if extraEnvs != nil {
		for _, e := range extraEnvs.Envs {
			b.addContainerEnvVar(e)
		}
	}
}
