package model

import (
	"fmt"
	"path/filepath"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/model/multiobject"
	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

type BackstagePvcsFactory struct{}

func (f BackstagePvcsFactory) newBackstageObject() RuntimeObject {
	__sealights__.TraceFunc("fc913ea333ceda0a37")
	return &BackstagePvcs{}
}

func init() {
	__sealights__.TraceFunc("2185de4099bede33b5")
	registerConfig("pvcs.yaml", BackstagePvcsFactory{}, true)
}

func addPvcsFromSpec(spec bsv1.BackstageSpec, model *BackstageModel) {
	__sealights__.TraceFunc("3d14eb437d01ea929a")
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

		addPvc(model.backstageDeployment, pvcSpec.Name, mountPath, subPath, []string{BackstageContainerName()})
	}
}

type BackstagePvcs struct {
	pvcs *multiobject.MultiObject
}

func PvcsName(backstageName, originalName string) string {
	__sealights__.TraceFunc("e05c3d3f8518274269")
	return fmt.Sprintf("%s-%s", utils.GenerateRuntimeObjectName(backstageName, "backstage"), originalName)
}

func (b *BackstagePvcs) Object() runtime.Object {
	__sealights__.TraceFunc("4c307cdb34cc5839e0")
	return b.pvcs
}

func (b *BackstagePvcs) setObject(object runtime.Object) {
	__sealights__.TraceFunc("2b15cf3e0275530542")
	b.pvcs = object.(*multiobject.MultiObject)
}

func (b *BackstagePvcs) EmptyObject() client.Object {
	__sealights__.TraceFunc("26a448a1cd40447b19")
	return &corev1.PersistentVolumeClaim{}
}

func (b *BackstagePvcs) addToModel(model *BackstageModel, _ bsv1.Backstage) (bool, error) {
	__sealights__.TraceFunc("9438072f261e57d027")
	if b.pvcs != nil {
		model.setRuntimeObject(b)
		return true, nil
	}
	return false, nil
}

func (b *BackstagePvcs) updateAndValidate(m *BackstageModel, _ bsv1.Backstage) error {
	__sealights__.TraceFunc("e855b5b5c6965c76d6")
	for _, o := range b.pvcs.Items {
		pvc, ok := o.(*corev1.PersistentVolumeClaim)
		if !ok {
			return fmt.Errorf("payload is not corev1.PersistentVolumeClaim: %T", o)
		}
		mountPath, subPath, containers := m.backstageDeployment.getDefConfigMountInfo(o)
		addPvc(m.backstageDeployment, pvc.Name, mountPath, subPath, containers)
	}
	return nil
}

func (b *BackstagePvcs) setMetaInfo(backstage bsv1.Backstage, scheme *runtime.Scheme) {
	__sealights__.TraceFunc("d9ee812514cd04a048")
	for _, item := range b.pvcs.Items {
		pvc := item.(*corev1.PersistentVolumeClaim)
		utils.AddAnnotation(pvc, ConfiguredNameAnnotation, item.GetName())
		pvc.Name = PvcsName(backstage.Name, pvc.Name)
		setMetaInfo(pvc, backstage, scheme)
	}
}

func addPvc(bsd *BackstageDeployment, pvcName, mountPath, subPath string, affectedContainers []string) {
	__sealights__.TraceFunc("5500180d4c47407c1c")

	volName := utils.ToRFC1123Label(pvcName)
	volSrc := corev1.VolumeSource{
		PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
			ClaimName: pvcName,
		},
	}
	bsd.deployment.Spec.Template.Spec.Volumes =
		append(bsd.deployment.Spec.Template.Spec.Volumes, corev1.Volume{Name: volName, VolumeSource: volSrc})
	for _, c := range affectedContainers {
		update := bsd.containerByName(c)
		update.VolumeMounts = append(update.VolumeMounts,
			corev1.VolumeMount{Name: volName, MountPath: mountPath, SubPath: subPath})
	}
}
