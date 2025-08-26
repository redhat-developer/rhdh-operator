package controller

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
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
