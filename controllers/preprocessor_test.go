package controller

import (
	"context"
	"os"
	"redhat-developer/red-hat-developer-hub-operator/api/v1alpha2"
	"redhat-developer/red-hat-developer-hub-operator/pkg/model"
	"testing"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func updateConfigMap(t *testing.T) BackstageReconciler {
	ctx := context.TODO()

	bs := v1alpha2.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs1",
			Namespace: "ns1",
		},
		Spec: v1alpha2.BackstageSpec{
			Application: &v1alpha2.Application{
				AppConfig: &v1alpha2.AppConfig{
					ConfigMaps: []v1alpha2.ObjectKeyRef{{Name: "cm1"}},
				},
			},
		},
	}

	cm := corev1.ConfigMap{}
	cm.Name = "cm1"

	rc := BackstageReconciler{
		Client: NewMockClient(),
	}

	assert.NoError(t, rc.Create(ctx, &cm))

	// reconcile
	extConf, err := rc.preprocessSpec(ctx, bs)
	assert.NoError(t, err)

	assert.NotNil(t, extConf.AppConfigs["cm1"].Labels)
	assert.Equal(t, 1, len(extConf.AppConfigs["cm1"].Labels))
	oldHash := extConf.GetHash()

	// Update ConfigMap with new data
	err = rc.Get(ctx, types.NamespacedName{Namespace: "ns1", Name: "cm1"}, &cm)
	assert.NoError(t, err)
	cm.Data = map[string]string{"key": "value"}
	err = rc.Update(ctx, &cm)
	assert.NoError(t, err)

	// reconcile again
	extConf, err = rc.preprocessSpec(ctx, bs)
	assert.NoError(t, err)

	assert.NotEqual(t, oldHash, extConf.GetHash())

	return rc
}

func TestExtConfigChanged(t *testing.T) {

	ctx := context.TODO()
	cm := corev1.ConfigMap{}

	rc := updateConfigMap(t)
	err := rc.Get(ctx, types.NamespacedName{Namespace: "ns1", Name: "cm1"}, &cm)
	assert.NoError(t, err)
	// true : Backstage will be reconciled
	assert.Equal(t, "true", cm.Labels[model.ExtConfigSyncLabel])

	err = os.Setenv(AutoSyncEnvVar, "false")
	assert.NoError(t, err)

	rc = updateConfigMap(t)
	err = rc.Get(ctx, types.NamespacedName{Namespace: "ns1", Name: "cm1"}, &cm)
	assert.NoError(t, err)
	// false : Backstage will not be reconciled
	assert.Equal(t, "false", cm.Labels[model.ExtConfigSyncLabel])

}
