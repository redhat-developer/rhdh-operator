package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/redhat-developer/rhdh-operator/pkg/model"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

type fakeDeleteClient struct {
	client.Client
	deleteFunc func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
}

func (f *fakeDeleteClient) Delete(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error {
	return f.deleteFunc(ctx, obj, opts...)
}

func TestBackstageReconciler_tryToDelete(t *testing.T) {
	tests := []struct {
		name       string
		deleteFunc func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) error
		wantErr    bool
	}{
		{
			name: "success",
			deleteFunc: func(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
				return nil
			},
			wantErr: false,
		},
		{
			name: "not found error",
			deleteFunc: func(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
				return apierrors.NewNotFound(
					schema.GroupResource{Group: "my-group", Resource: "my-resource"},
					"some-name",
				)
			},
			wantErr: false,
		},
		{
			// RHDHBUGS-1990
			name: "no match error",
			deleteFunc: func(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
				return &meta.NoKindMatchError{
					GroupKind: schema.GroupKind{Group: "monitoring.coreos.com", Kind: "ServiceMonitor"},
				}
			},
			wantErr: false,
		},
		{
			name: "other error",
			deleteFunc: func(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
				return fmt.Errorf("any other unexpected error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &BackstageReconciler{
				Client: &fakeDeleteClient{
					deleteFunc: tt.deleteFunc,
				},
			}
			err := r.tryToDelete(context.TODO(), &unstructured.Unstructured{}, "my-name", "my-ns")

			if tt.wantErr {
				assert.Error(t, err, "expected an error but got none")
			} else {
				assert.NoError(t, err, "expected no error but got one")
			}
		})
	}
}

type fakeApplyClient struct {
	client.Client
	getFunc   func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error
	patchFunc func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
}

func (f *fakeApplyClient) Get(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error {
	if f.getFunc == nil {
		return fmt.Errorf("unexpected Get call")
	}
	return f.getFunc(ctx, key, obj, opts...)
}

func (f *fakeApplyClient) Patch(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
	if f.patchFunc == nil {
		return fmt.Errorf("unexpected Patch call")
	}
	return f.patchFunc(ctx, obj, patch, opts...)
}

func TestBackstageReconciler_applyPayload_preventOverwrite(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)

	baseObj := &corev1.ConfigMap{
		TypeMeta: metav1.TypeMeta{APIVersion: "v1", Kind: "ConfigMap"},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-configmap",
			Namespace: "my-ns",
		},
	}

	tests := []struct {
		name               string
		desiredAnnotations map[string]string
		getFunc            func(ctx context.Context, key types.NamespacedName, obj client.Object, opts ...client.GetOption) error
		patchFunc          func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error
		patchCalled        bool
		wantErr            bool
		wantErrContains    string
	}{
		{
			name: "skips apply when prevent-overwrite annotation is present",
			getFunc: func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
				obj.SetAnnotations(map[string]string{model.PreventOverwriteAnnotation: "true"})
				return nil
			},
			patchCalled: false,
			wantErr:     false,
		},
		{
			name: "skips apply when prevent-overwrite annotation is capitalized True",
			getFunc: func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
				obj.SetAnnotations(map[string]string{model.PreventOverwriteAnnotation: "True"})
				return nil
			},
			patchCalled: false,
			wantErr:     false,
		},
		{
			name: "proceeds with apply when prevent-overwrite annotation is false",
			getFunc: func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
				obj.SetAnnotations(map[string]string{model.PreventOverwriteAnnotation: "false"})
				return nil
			},
			patchCalled: true,
			wantErr:     false,
		},
		{
			name: "proceeds with apply when prevent-overwrite annotation is empty",
			getFunc: func(_ context.Context, _ types.NamespacedName, obj client.Object, _ ...client.GetOption) error {
				obj.SetAnnotations(map[string]string{model.PreventOverwriteAnnotation: ""})
				return nil
			},
			patchCalled: true,
			wantErr:     false,
		},
		{
			name: "proceeds with apply when annotation is absent",
			getFunc: func(_ context.Context, _ types.NamespacedName, _ client.Object, _ ...client.GetOption) error {
				return nil
			},
			patchCalled: true,
			wantErr:     false,
		},
		{
			name:               "proceeds with apply when only desired object has prevent-overwrite annotation",
			desiredAnnotations: map[string]string{model.PreventOverwriteAnnotation: "true"},
			getFunc: func(_ context.Context, _ types.NamespacedName, _ client.Object, _ ...client.GetOption) error {
				return apierrors.NewNotFound(
					schema.GroupResource{Group: "", Resource: "configmaps"},
					"my-configmap",
				)
			},
			patchCalled: true,
			wantErr:     false,
		},
		{
			name: "proceeds with apply when object does not exist yet",
			getFunc: func(_ context.Context, _ types.NamespacedName, _ client.Object, _ ...client.GetOption) error {
				return apierrors.NewNotFound(
					schema.GroupResource{Group: "", Resource: "configmaps"},
					"my-configmap",
				)
			},
			patchCalled: true,
			wantErr:     false,
		},
		{
			name: "returns error on transient Get failure",
			getFunc: func(_ context.Context, _ types.NamespacedName, _ client.Object, _ ...client.GetOption) error {
				return fmt.Errorf("transient API timeout")
			},
			patchCalled:     false,
			wantErr:         true,
			wantErrContains: "failed to get object for prevent-overwrite check",
		},
		{
			name: "returns error when apply patch fails",
			getFunc: func(_ context.Context, _ types.NamespacedName, _ client.Object, _ ...client.GetOption) error {
				return nil
			},
			patchFunc: func(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
				return fmt.Errorf("server-side apply failed")
			},
			patchCalled:     true,
			wantErr:         true,
			wantErrContains: "failed to apply object",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			patched := false
			patchFunc := tt.patchFunc
			if patchFunc == nil {
				patchFunc = func(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
					patched = true
					return nil
				}
			} else {
				wrappedPatchFunc := patchFunc
				patchFunc = func(ctx context.Context, obj client.Object, patch client.Patch, opts ...client.PatchOption) error {
					patched = true
					return wrappedPatchFunc(ctx, obj, patch, opts...)
				}
			}

			r := &BackstageReconciler{
				Client: &fakeApplyClient{
					getFunc: tt.getFunc,
					patchFunc: patchFunc,
				},
				Scheme: scheme,
			}
			obj := baseObj.DeepCopy()
			if tt.desiredAnnotations != nil {
				obj.SetAnnotations(tt.desiredAnnotations)
			}
			err := r.applyPayload(context.TODO(), obj, false)

			if tt.wantErr {
				assert.Error(t, err)
				if tt.wantErrContains != "" {
					assert.ErrorContains(t, err, tt.wantErrContains)
				}
			} else {
				assert.NoError(t, err)
			}
			assert.Equal(t, tt.patchCalled, patched, "unexpected patch call state")
		})
	}
}
