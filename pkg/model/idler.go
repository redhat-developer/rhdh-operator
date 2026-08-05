package model

import "github.com/redhat-developer/rhdh-operator/api"

// Idler is implemented by RuntimeObjects whose workloads can be scaled to
// zero when the Backstage CR carries the idle annotation.
type Idler interface {
	Idle()
}

// ShouldIdle reports whether the Backstage CR requests idling.
func ShouldIdle(backstage api.Backstage) bool {
	return backstage.GetAnnotations()[IdleAnnotation] == "true"
}
