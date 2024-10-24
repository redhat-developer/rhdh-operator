package model

import (
	"fmt"
	"path/filepath"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	appsv1 "k8s.io/api/apps/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type BackstagePvcsFactory struct{}

func (f BackstagePvcsFactory) newBackstageObject() RuntimeObject {
	return &BackstagePvcs{mountPath: DefaultMountDir, fromDefaultConf: true}
}

func init() {
	registerConfig("pvcs.yaml", BackstagePvcsFactory{}, true)
}

func addPvc(spec bsv1.BackstageSpec, deployment *appsv1.Deployment, model *BackstageModel) {
	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.Pvcs == nil || len(spec.Application.ExtraFiles.Pvcs) == 0 {
		return
	}

	mp := DefaultMountDir
	if spec.Application.ExtraFiles.MountPath != "" {
		mp = spec.Application.ExtraFiles.MountPath
	}

	for _, pvcSpec := range spec.Application.ExtraFiles.Pvcs {
		pvc, ok := model.ExternalConfig.ExtraPvcs[pvcSpec.Name]
		if ok {
			pvcObj := BackstagePvcs{fromDefaultConf: false}
			pvcObj.pvcs = &multiobject.MultiObject{}
			if pvcSpec.MountPath == "" {
				pvcObj.mountPath = mp
				pvcObj.fullPath = false

			} else {
				pvcObj.mountPath = pvcSpec.MountPath
				pvcObj.fullPath = true
			}
			pvcObj.pvcs.Items = append(pvcObj.pvcs.Items, &pvc)
			pvcObj.updatePod(deployment)
		}
	}
}

type BackstagePvcs struct {
	pvcs      *multiobject.MultiObject
	mountPath string
	// if false mountPath will be concatenated with pvc name
	fullPath bool
	// whether this object is constructed from default config
	fromDefaultConf bool
}

func PvcsName(backstageName, originalName string) string {
	return fmt.Sprintf("%s-%s", utils.GenerateRuntimeObjectName(backstageName, "backstage"), originalName)
}

func (b *BackstagePvcs) Object() runtime.Object {
	return b.pvcs
}

func (b *BackstagePvcs) setObject(object runtime.Object) {
	b.pvcs = object.(*multiobject.MultiObject)
}

func (b *BackstagePvcs) EmptyObject() client.Object {
	return &corev1.PersistentVolumeClaim{}
}

func (b *BackstagePvcs) addToModel(model *BackstageModel, backstage bsv1.Backstage) (bool, error) {
	if b.pvcs != nil {
		model.setRuntimeObject(b)
		return true, nil
	}
	return false, nil
}

func (b *BackstagePvcs) validate(model *BackstageModel, backstage bsv1.Backstage) error {
	for _, o := range b.pvcs.Items {
		_, ok := o.(*corev1.PersistentVolumeClaim)
		if !ok {
			return fmt.Errorf("payload is not corev1.PersistentVolumeClaim: %T", o)
		}

	}
	return nil
}

func (b *BackstagePvcs) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	for _, item := range b.pvcs.Items {
		pvc := item.(*corev1.PersistentVolumeClaim)
		utils.AddAnnotation(pvc, ConfiguredNameAnnotation, item.GetName())
		pvc.Name = PvcsName(backstage.Name, pvc.Name)
		setMetaInfo(pvc, backstage, scheme)
	}
}

// implementation of BackstagePodContributor interface
func (b *BackstagePvcs) updatePod(deployment *appsv1.Deployment) {

	for _, pvc := range b.pvcs.Items {
		volName := utils.ToRFC1123Label(pvc.GetName())
		volSrc := corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvc.GetName(),
			},
		}
		deployment.Spec.Template.Spec.Volumes =
			append(deployment.Spec.Template.Spec.Volumes, corev1.Volume{Name: volName, VolumeSource: volSrc})

		c := &deployment.Spec.Template.Spec.Containers[0]
		volMount := corev1.VolumeMount{Name: volName}

		volMount.MountPath = filepath.Join(b.mountPath, volName)
		if mp, ok := pvc.GetAnnotations()[DefaultMountPathAnnotation]; ok && b.fromDefaultConf {
			volMount.MountPath = mp
		}
		if b.fullPath && !b.fromDefaultConf {
			volMount.MountPath = b.mountPath
		}

		c.VolumeMounts = append(c.VolumeMounts, volMount)

	}
}
