package controller

import (
	"context"
	"fmt"

	"github.com/redhat-developer/rhdh-operator/pkg/model"
	appsv1 "k8s.io/api/apps/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

func FindDeployment(ctx context.Context, k8sClient client.Client, namespace, backstageName string) (model.Deployable, error) {
	nn := client.ObjectKey{Namespace: namespace, Name: model.DeploymentName(backstageName)}
	deploy := &appsv1.Deployment{}
	lg := log.FromContext(ctx)
	lg.V(1).Info("Looking for Deployment", "namespace", namespace, "name", nn.Name)
	err := k8sClient.Get(ctx, nn, deploy)
	if err == nil {
		return model.CreateDeployable(deploy)
	} else if errors.IsNotFound(err) {
		ss := &appsv1.StatefulSet{}
		lg.V(1).Info("Deployment not found, looking for StatefulSet", "namespace", namespace, "name", nn.Name)
		err = k8sClient.Get(ctx, nn, ss)
		if err == nil {
			return model.CreateDeployable(ss)
		}
	}
	if errors.IsNotFound(err) {
		return nil, fmt.Errorf("neither Deployment nor StatefulSet found for Backstage %s in namespace %s", backstageName, namespace)
	}
	return nil, err
}
