package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

	__sealights__ "github.com/redhat-developer/rhdh-operator/__sealights__"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const implementMe = "implement me if needed"

// Mock K8s go-client with very basic implementation of (some) methods
// to be able to simply test controller logic
type MockClient struct {
	objects map[NameKind][]byte
}

func NewMockClient() MockClient {
	__sealights__.TraceFunc("c276d0de8e088b9f7d")
	return MockClient{
		objects: map[NameKind][]byte{},
	}
}

type NameKind struct {
	Name string
	Kind string
}

func kind(obj runtime.Object) string {
	__sealights__.TraceFunc("c9bc8d408b3f50a996")
	str := reflect.TypeOf(obj).String()
	return str[strings.LastIndex(str, ".")+1:]
	//return reflect.TypeOf(obj).String()
}

func (m MockClient) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {
	__sealights__.TraceFunc("4bc469382c70167f29")

	if key.Name == "" {
		return fmt.Errorf("get: name should not be empty")
	}
	uobj := m.objects[NameKind{Name: key.Name, Kind: kind(obj)}]
	if uobj == nil {
		return errors.NewNotFound(schema.GroupResource{Group: "", Resource: kind(obj)}, key.Name)
	}
	err := json.Unmarshal(uobj, obj)
	if err != nil {
		return err
	}
	return nil
}

func (m MockClient) List(_ context.Context, _ client.ObjectList, _ ...client.ListOption) error {
	__sealights__.TraceFunc("70bdbb2e2fe44c606e")
	panic(implementMe)
}

func (m MockClient) Create(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
	__sealights__.TraceFunc("a229aa024e6edd351e")
	if obj.GetName() == "" {
		return fmt.Errorf("update: object Name should not be empty")
	}
	uobj := m.objects[NameKind{Name: obj.GetName(), Kind: kind(obj)}]
	if uobj != nil {
		return errors.NewAlreadyExists(schema.GroupResource{Group: "", Resource: kind(obj)}, obj.GetName())
	}
	dat, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	m.objects[NameKind{Name: obj.GetName(), Kind: kind(obj)}] = dat
	return nil
}

func (m MockClient) Delete(_ context.Context, _ client.Object, _ ...client.DeleteOption) error {
	__sealights__.TraceFunc("a6d2d167efbd48ea30")
	panic(implementMe)
}

func (m MockClient) Update(_ context.Context, obj client.Object, _ ...client.UpdateOption) error {
	__sealights__.TraceFunc("da9f847c7450e2b862")

	if obj.GetName() == "" {
		return fmt.Errorf("update: object Name should not be empty")
	}
	uobj := m.objects[NameKind{Name: obj.GetName(), Kind: kind(obj)}]
	if uobj == nil {
		return errors.NewNotFound(schema.GroupResource{Group: "", Resource: kind(obj)}, obj.GetName())
	}
	dat, err := json.Marshal(obj)
	if err != nil {
		return err
	}
	m.objects[NameKind{Name: obj.GetName(), Kind: kind(obj)}] = dat
	return nil
}

func (m MockClient) Patch(_ context.Context, _ client.Object, _ client.Patch, _ ...client.PatchOption) error {
	__sealights__.TraceFunc("0957bb9abb712a090d")
	panic(implementMe)
}

func (m MockClient) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	__sealights__.TraceFunc("c75ab2d609bec6041e")
	panic(implementMe)
}

func (m MockClient) Status() client.SubResourceWriter {
	__sealights__.TraceFunc("7c814deb0b4f241a6b")
	panic(implementMe)
}

func (m MockClient) SubResource(_ string) client.SubResourceClient {
	__sealights__.TraceFunc("5c4ec026686aa23521")
	panic(implementMe)
}

func (m MockClient) Scheme() *runtime.Scheme {
	__sealights__.TraceFunc("9daa231f88edb3bb32")
	panic(implementMe)
}

func (m MockClient) RESTMapper() meta.RESTMapper {
	__sealights__.TraceFunc("924ae939bbeb8fdc68")
	panic(implementMe)
}

func (m MockClient) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	__sealights__.TraceFunc("d6cf60a610048dc5e9")
	panic(implementMe)
}

func (m MockClient) IsObjectNamespaced(_ runtime.Object) (bool, error) {
	__sealights__.TraceFunc("d5ab56102ed0bb1c80")
	panic(implementMe)
}
