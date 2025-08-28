package model

import (
	"fmt"
	"path/filepath"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha4"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type BackstagePvcsFactory struct{}

func (f BackstagePvcsFactory) newBackstageObject() RuntimeObject {
	return &BackstagePvcs{}
}

func init() {
	registerConfig("pvcs.yaml", BackstagePvcsFactory{}, true)
}

func (b *BackstagePvcs) addExternalConfig(spec bsv1.BackstageSpec) error {
	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.Pvcs == nil || len(spec.Application.ExtraFiles.Pvcs) == 0 {
		return nil
	}

	for _, pvcSpec := range spec.Application.ExtraFiles.Pvcs {

		subPath := ""
		mountPath, wSubpath := b.model.backstageDeployment.mountPath(pvcSpec.MountPath, "", spec.Application.ExtraFiles.MountPath)

		if wSubpath {
			mountPath = filepath.Join(mountPath, pvcSpec.Name)
			subPath = utils.ToRFC1123Label(pvcSpec.Name)
		}

		err := addPvc(b.model.backstageDeployment, pvcSpec.Name, mountPath, subPath, containersFilter{names: pvcSpec.Containers})
		if err != nil {
			return fmt.Errorf("failed to add pvc %s: %w", pvcSpec.Name, err)
		}
	}
	return nil
}

type BackstagePvcs struct {
	pvcs  *multiobject.MultiObject
	model *BackstageModel
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

func (b *BackstagePvcs) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	b.model = model
	if b.pvcs != nil {
		model.setRuntimeObject(b)
		return true, nil
	}
	return false, nil
}

func (b *BackstagePvcs) updateAndValidate(_ bsv1.Backstage) error {
	for _, o := range b.pvcs.Items {
		pvc, ok := o.(*corev1.PersistentVolumeClaim)
		if !ok {
			return fmt.Errorf("payload is not corev1.PersistentVolumeClaim: %T", o)
		}
		mountPath, subPath := b.model.backstageDeployment.getDefConfigMountPath(o)
		err := addPvc(b.model.backstageDeployment, pvc.Name, mountPath, subPath, containersFilter{annotation: o.GetAnnotations()[ContainersAnnotation]})
		if err != nil {
			return fmt.Errorf("failed to get containers for pvc %s: %w", o.GetName(), err)
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

func addPvc(bsd *BackstageDeployment, pvcName, mountPath, subPath string, filter containersFilter) error {

	volName := utils.ToRFC1123Label(pvcName)
	volSrc := corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: pvcName,
		},
	}
	bsd.deployment.Spec.Template.Spec.Volumes =
		append(bsd.deployment.Spec.Template.Spec.Volumes, corev1.Volume{Name: volName, VolumeSource: volSrc})
	affectedContainers, err := filter.getContainers(bsd)
	if err != nil {
		return fmt.Errorf("failed to mount files for pvc %s: %w", pvcName, err)
	}
	for _, c := range affectedContainers {
		//update := bsd.containerByName(c)
		c.VolumeMounts = append(c.VolumeMounts,
			corev1.VolumeMount{Name: volName, MountPath: mountPath, SubPath: subPath})
	}
	return nil
}
