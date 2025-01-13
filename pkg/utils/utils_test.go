package utils

import (
	"testing"

	appsv1 "k8s.io/api/apps/v1"

	openshift "github.com/openshift/api/route/v1"

	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

var util_test_scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(util_test_scheme))
}

func TestToRFC1123Label(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		// The inputs below are all valid names for K8s ConfigMaps or Secrets.

		{
			name: "should replace invalid characters with a dash",
			in:   "kube-root-ca.crt",
			want: "kube-root-ca-crt",
		},
		{
			name: "all-numeric string should remain unchanged",
			in:   "123456789",
			want: "123456789",
		},
		{
			name: "should truncate up to the maximum length and remove leading and trailing dashes",
			in:   "ppxkgq.df-yyatvyrgjtwivunibicne-bvyyotvonbrtfv-awylmrez.ksvqjw-z.xpgdi", /* 70 characters */
			want: "ppxkgq-df-yyatvyrgjtwivunibicne-bvyyotvonbrtfv-awylmrez-ksvqjw",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ToRFC1123Label(tt.in); got != tt.want {
				t.Errorf("ToRFC1123Label() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestReadMultiObject(t *testing.T) {

	y := `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: pvc1
spec:
  storageClassName: local-storage
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: pvc2
spec:
  storageClassName: local-storage2
  accessModes:
     - ReadWriteOnce
  resources:
     requests:
       storage: 2Gi`

	objects, err := ReadYamls([]byte(y), nil, &corev1.PersistentVolumeClaim{}, *util_test_scheme)

	assert.NoError(t, err)

	assert.Equal(t, 2, len(objects))
	assert.Equal(t, "pvc1", objects[0].(*corev1.PersistentVolumeClaim).GetName())

}

func TestReadMultiInvalidObject(t *testing.T) {

	y := `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: pvc1
spec:
  storageClassName: local-storage
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 1Gi
---
apiVersion: v1
kind: ConfigMap
metadata:
  name: pvc2
data:`

	_, err := ReadYamls([]byte(y), nil, &corev1.PersistentVolumeClaim{}, *util_test_scheme)

	// Kind not match for the second item, PersistentVolumeClaim expected
	assert.EqualError(t, err, "GroupVersionKind not match, found: /v1, Kind=ConfigMap, expected: [/v1, Kind=PersistentVolumeClaim]")

}

func TestPlatformPatchMerge(t *testing.T) {

	// ocp (no patch, so default)
	t.Setenv("TEST_PLATFORM", "ocp")
	obj, err := ReadYamlFiles("testdata/deployment.yaml", &appsv1.Deployment{}, *util_test_scheme)
	assert.NoError(t, err)

	depl, ok := obj[0].(*appsv1.Deployment)
	assert.Equal(t, true, ok)
	assert.Equal(t, int32(1), *depl.Spec.Replicas)

	// k8s (patched)
	t.Setenv("TEST_PLATFORM", "k8s")

	obj, err = ReadYamlFiles("testdata/deployment.yaml", &appsv1.Deployment{}, *util_test_scheme)
	assert.NoError(t, err)

	depl, ok = obj[0].(*appsv1.Deployment)
	assert.Equal(t, true, ok)
	assert.Equal(t, int32(2), *depl.Spec.Replicas)

}

func TestGetObjectKind(t *testing.T) {

	objk := GetObjectKind(&corev1.PersistentVolumeClaim{}, util_test_scheme)
	assert.NotNil(t, objk)
	assert.Equal(t, "PersistentVolumeClaim", objk.Kind)
	assert.Equal(t, "v1", objk.Version)

	// should fail since openshift scheme is not registered for this test
	objk = GetObjectKind(&openshift.Route{}, util_test_scheme)
	assert.Nil(t, objk)

}

func TestBoolEnvVar(t *testing.T) {

	// not defined - true
	assert.True(t, BoolEnvVar("MyVar", true))
	// false
	t.Setenv("MyVar", "false")
	assert.False(t, BoolEnvVar("MyVar", true))
	// anything else than eligible to false - true
	t.Setenv("MyVar", "anything")
	assert.True(t, BoolEnvVar("anything", true))
}

func TestFilterContainers(t *testing.T) {

	containers := []corev1.Container{{Name: "c1"}, {Name: "c2"}, {Name: "c3"}}

	cs := FilterContainers(containers, "")
	assert.Nil(t, cs)

	cs = FilterContainers(containers, "*")
	assert.Equal(t, 3, len(cs))

	cs = FilterContainers(containers, "c123")
	assert.Equal(t, 0, len(cs))

	cs = FilterContainers(containers, "c1,c2")
	assert.Equal(t, 2, len(cs))
	assert.Equal(t, "c1", cs[0].Name)

}
