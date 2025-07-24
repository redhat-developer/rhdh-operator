package controller

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"

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
	return MockClient{
		objects: map[NameKind][]byte{},
	}
}

type NameKind struct {
	Name string
	Kind string
}

func kind(obj runtime.Object) string {
	str := reflect.TypeOf(obj).String()
	return str[strings.LastIndex(str, ".")+1:]
	//return reflect.TypeOf(obj).String()
}

func (m MockClient) Get(_ context.Context, key client.ObjectKey, obj client.Object, _ ...client.GetOption) error {

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
	panic(implementMe)
}

func (m MockClient) Create(_ context.Context, obj client.Object, _ ...client.CreateOption) error {
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

func (m MockClient) Delete(_ context.Context, obj client.Object, _ ...client.DeleteOption) error {
	// Remove the object from the mock store
	key := NameKind{Name: obj.GetName(), Kind: kind(obj)}
	if _, exists := m.objects[key]; !exists {
		return errors.NewNotFound(schema.GroupResource{Group: "", Resource: kind(obj)}, obj.GetName())
	}
	delete(m.objects, key)
	return nil
}

func (m MockClient) Update(_ context.Context, obj client.Object, _ ...client.UpdateOption) error {

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
	panic(implementMe)
}

func (m MockClient) DeleteAllOf(_ context.Context, _ client.Object, _ ...client.DeleteAllOfOption) error {
	panic(implementMe)
}

func (m MockClient) Status() client.SubResourceWriter {
	panic(implementMe)
}

func (m MockClient) SubResource(_ string) client.SubResourceClient {
	panic(implementMe)
}

func (m MockClient) Scheme() *runtime.Scheme {
	panic(implementMe)
}

func (m MockClient) RESTMapper() meta.RESTMapper {
	panic(implementMe)
}

func (m MockClient) GroupVersionKindFor(_ runtime.Object) (schema.GroupVersionKind, error) {
	panic(implementMe)
}

func (m MockClient) IsObjectNamespaced(_ runtime.Object) (bool, error) {
	panic(implementMe)
}
