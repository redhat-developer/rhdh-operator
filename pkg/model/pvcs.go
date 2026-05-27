package model

import (
	"fmt"
	"path/filepath"

	"github.com/redhat-developer/rhdh-operator/api"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type BackstagePvcsFactory struct{}

func (f BackstagePvcsFactory) newBackstageObject() RuntimeObject {
	return &BackstagePvcs{}
}

func init() {
	registerConfig(PvcsKey, BackstagePvcsFactory{}, true, nil)
}

type BackstagePvcs struct {
	pvcs  *multiobject.MultiObject
	model *BackstageModel
}

func (b *BackstagePvcs) Object() runtime.Object {
	if b.pvcs != nil && len(b.pvcs.Items) > 0 {
		return b.pvcs
	}
	return nil
}

// implementation of RuntimeObject interface
func (b *BackstagePvcs) GetKey() string {
	return PvcsKey
}

func (b *BackstagePvcs) addToModel(model *BackstageModel, backstage api.Backstage, config runtime.Object, scheme *runtime.Scheme) error {
	b.model = model
	if config != nil {
		b.pvcs = config.(*multiobject.MultiObject)
	}

	// Always add wrapper to model (unconditional)
	model.setRuntimeObject(b)

	// Only set metadata if underlying object exists
	if b.pvcs != nil && len(b.pvcs.Items) > 0 {
		b.setMetaInfo(backstage, scheme)
	}
	return nil
}

func (b *BackstagePvcs) updateAndValidate(backstage api.Backstage, scheme *runtime.Scheme) error {
	deployment := b.model.getDeployment()
	if deployment == nil {
		return fmt.Errorf("backstage deployment not found in model")
	}

	// Process PVCs from config files
	if b.pvcs != nil {
		for _, o := range b.pvcs.Items {
			pvc, ok := o.(*corev1.PersistentVolumeClaim)
			if !ok {
				return fmt.Errorf("payload is not corev1.PersistentVolumeClaim: %T", o)
			}
			mountPath, subPath, _ := deployment.getDefConfigMountPath(o)
			err := addPvc(deployment, pvc.Name, mountPath, subPath, containersFilter{annotation: o.GetAnnotations()[ContainersAnnotation]})
			if err != nil {
				return fmt.Errorf("failed to get containers for pvc %s: %w", o.GetName(), err)
			}
		}
	}

	// Process PVCs from CR spec (formerly addExternalConfig)
	if backstage.Spec.Application != nil && backstage.Spec.Application.ExtraFiles != nil && backstage.Spec.Application.ExtraFiles.Pvcs != nil && len(backstage.Spec.Application.ExtraFiles.Pvcs) > 0 {
		for _, pvcSpec := range backstage.Spec.Application.ExtraFiles.Pvcs {

			subPath := ""
			mountPath, wSubpath := deployment.mountPath(pvcSpec.MountPath, "", backstage.Spec.Application.ExtraFiles.MountPath)

			if wSubpath {
				mountPath = filepath.Join(mountPath, pvcSpec.Name)
				subPath = utils.ToRFC1123Label(pvcSpec.Name)
			}

			err := addPvc(deployment, pvcSpec.Name, mountPath, subPath, containersFilter{names: pvcSpec.Containers})
			if err != nil {
				return fmt.Errorf("failed to add pvc %s: %w", pvcSpec.Name, err)
			}
		}
	}

	return nil
}

func (b *BackstagePvcs) setMetaInfo(backstage api.Backstage, scheme *runtime.Scheme) {
	setMultiObjectConfigMetaInfo(b.pvcs, "pvcs", backstage, scheme)
}

func addPvc(bsd *BackstageDeployment, pvcName, mountPath, subPath string, filter containersFilter) error {

	volName := utils.ToRFC1123Label(pvcName)
	volSrc := corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: pvcName,
		},
	}
	bsd.podSpec().Volumes =
		append(bsd.podSpec().Volumes, corev1.Volume{Name: volName, VolumeSource: volSrc})
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
