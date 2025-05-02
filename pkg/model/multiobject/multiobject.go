package multiobject

import (
	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
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
	__sealights__.TraceFunc("9d3a33458da4093b05")
	return m.ObjectKind
}

func (m *MultiObject) DeepCopyObject() runtime.Object {
	__sealights__.TraceFunc("7ff8919faa50a1e17a")
	panic("DeepCopyObject for MultiObject is not implemented")
}
