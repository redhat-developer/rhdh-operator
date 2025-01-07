package controller

import (
	"context"
	"os"

	"testing"

	bsv1 "github.com/redhat-developer/rhdh-operator/api/v1alpha3"
	"github.com/redhat-developer/rhdh-operator/pkg/model"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
)

func updateConfigMap(t *testing.T) BackstageReconciler {
	ctx := context.TODO()

	bs := bsv1.Backstage{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "bs1",
			Namespace: "ns1",
		},
		Spec: bsv1.BackstageSpec{
			Application: &bsv1.Application{
				AppConfig: &bsv1.AppConfig{
					ConfigMaps: []bsv1.FileObjectRef{{Name: "cm1"}},
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

	oldHash := extConf.WatchingHash

	// Update ConfigMap with new data
	err = rc.Get(ctx, types.NamespacedName{Namespace: "ns1", Name: "cm1"}, &cm)
	assert.NoError(t, err)
	cm.Data = map[string]string{"key": "value"}
	err = rc.Update(ctx, &cm)
	assert.NoError(t, err)

	// reconcile again
	extConf, err = rc.preprocessSpec(ctx, bs)
	assert.NoError(t, err)

	assert.NotEqual(t, oldHash, extConf.WatchingHash)

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

// TestExtConfigChanged tests if concatData returns the same data when the order of the keys is different
func TestExtConcatData(t *testing.T) {
	cm := corev1.ConfigMap{}

	cm.Data = map[string]string{"key1": "value1", "key2": "value2", "key3": "value3", "key4": "value4"}
	original := []byte("original")
	data1 := concatData(original, &cm)

	cm.Data = map[string]string{"key4": "value4", "key2": "value2", "key3": "value3", "key1": "value1"}
	assert.Equal(t, data1, concatData(original, &cm))

}
