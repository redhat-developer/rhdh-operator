package multiobject

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// MultiObject implements runtime.Object interface to make it used in the model along with client.Object
type MultiObject struct {
	ObjectKind schema.ObjectKind
	Items      []client.Object
}

func (m *MultiObject) GetObjectKind() schema.ObjectKind {
	return m.ObjectKind
}

func (m *MultiObject) DeepCopyObject() runtime.Object {
	panic("DeepCopyObject for MultiObject is not implemented")
}
