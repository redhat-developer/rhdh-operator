package model

import (
	"fmt"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"
	corev1 "k8s.io/api/core/v1"
)

type containersFilter struct {
	names      []string
	annotation string
}

// getContainers returns a list of containers filtered by the names provided in the names array or by a string annotation containing comma-separated names
// NOTE: If the annotation is not empty, it overrides the names in the struct, so, DO NOT set both.
// Empty annotation or names returns the main container.
// If the annotation or names[0] is "*"  it returns all containers.
// If a container name is not found in deployment, it returns an error.
func (f *containersFilter) getContainers(deployment *BackstageDeployment) ([]*corev1.Container, error) {

	if f.annotation != "" {
		f.names = utils.ParseCommaSeparated(f.annotation)
	}

	if f.names == nil || len(f.names) == 0 {
		return []*corev1.Container{deployment.container()}, nil
	}

	var filter = f.names
	if len(f.names) == 1 && f.names[0] == "*" {
		filter = deployment.allContainers()
	}

	containers := []*corev1.Container{}
	for _, c := range filter {
		container := deployment.containerByName(c)
		if container == nil {
			return nil, fmt.Errorf("container %s not found", c)
		}
		containers = append(containers, container)
	}
	return containers, nil
}
