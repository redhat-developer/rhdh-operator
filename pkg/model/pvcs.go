package model

import (
	"fmt"
	"path/filepath"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
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

func addPvcsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) {
	if spec.Application == nil || spec.Application.ExtraFiles == nil || spec.Application.ExtraFiles.Pvcs == nil || len(spec.Application.ExtraFiles.Pvcs) == 0 {
		return
	}

	for _, pvcSpec := range spec.Application.ExtraFiles.Pvcs {

		subPath := ""
		mountPath, wSubpath := model.backstageDeployment.mountPath(pvcSpec.MountPath, "", spec.Application.ExtraFiles.MountPath)

		if wSubpath {
			mountPath = filepath.Join(mountPath, pvcSpec.Name)
			subPath = utils.ToRFC1123Label(pvcSpec.Name)
		}

		addPvc(model.backstageDeployment, pvcSpec.Name, mountPath, subPath, nil)
	}
}

type BackstagePvcs struct {
	pvcs *multiobject.MultiObject
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
	if b.pvcs != nil {
		model.setRuntimeObject(b)
		return true, nil
	}
	return false, nil
}

func (b *BackstagePvcs) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {
	for _, o := range b.pvcs.Items {
		pvc, ok := o.(*corev1.PersistentVolumeClaim)
		if !ok {
			return fmt.Errorf("payload is not corev1.PersistentVolumeClaim: %T", o)
		}
		mountPath, ok := pvc.GetAnnotations()[DefaultMountPathAnnotation]
		subPath := ""
		if !ok {
			volName := utils.ToRFC1123Label(pvc.GetName())
			mountPath = filepath.Join(m.backstageDeployment.defaultMountPath(), volName)
			subPath = volName
		}

		containers := utils.FilterContainers(m.backstageDeployment.allContainers(), pvc.GetAnnotations()[ContainersAnnotation])
		addPvc(m.backstageDeployment, pvc.Name, mountPath, subPath, containers)
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

func addPvc(bsd *BackstageDeployment, pvcName, mountPath, subPath string, affectedContainers []corev1.Container) {

	volName := utils.ToRFC1123Label(pvcName)
	volSrc := corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: pvcName,
		},
	}
	bsd.deployment.Spec.Template.Spec.Volumes =
		append(bsd.deployment.Spec.Template.Spec.Volumes, corev1.Volume{Name: volName, VolumeSource: volSrc})

	if affectedContainers == nil {
		// if nothing specified mount to the Backstage container only
		bsd.container().VolumeMounts = append(bsd.container().VolumeMounts,
			corev1.VolumeMount{Name: volName, MountPath: mountPath, SubPath: subPath})
	} else {
		// else mount to the affectedContainers
		for _, c := range affectedContainers {
			update := bsd.containerByName(c.Name)
			update.VolumeMounts = append(update.VolumeMounts,
				corev1.VolumeMount{Name: volName, MountPath: mountPath, SubPath: subPath})
		}
	}
}
