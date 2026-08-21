package controller

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOkpServiceURL(t *testing.T) {
	// In-cluster Service DNS when no ingress domain is known.
	assert.Equal(t,
		"http://okp.ns.svc.cluster.local:8080",
		okpServiceURL("okp", "ns", ""))

	// Route hostname when an OpenShift ingress domain is known.
	assert.Equal(t,
		"http://okp-ns.apps.example.com",
		okpServiceURL("okp", "ns", "apps.example.com"))
}
