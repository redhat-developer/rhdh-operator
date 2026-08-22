package model

import (
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/redhat-developer/rhdh-operator/pkg/utils"
)

// okpComponentName is the shared name/label value for OKP (Offline Knowledge Portal) resources.
const okpComponentName = "lightspeed-okp"

// OkpName returns the runtime object name for the OKP resources belonging to a Backstage CR.
// It must stay consistent with the URL derived in the controller (OKP_SERVICE_URL injection),
// otherwise the lightspeed-core sidecar would point at a non-existent Service/Route.
func OkpName(backstageName string) string {
	return utils.GenerateRuntimeObjectName(backstageName, okpComponentName)
}

func okpLabels(backstageName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":      okpComponentName,
		"app.kubernetes.io/instance":  backstageName,
		"app.kubernetes.io/component": okpComponentName,
	}
}

func okpSelectorLabels(backstageName string) map[string]string {
	return map[string]string{
		"app.kubernetes.io/name":     okpComponentName,
		"app.kubernetes.io/instance": backstageName,
	}
}

// mergeOkpObject sources a single standalone OKP object from the flavour config.
//
// OKP manifests live only in the lightspeed flavour directory (there is no base
// default-config entry), so a MergeFunc is required for ReadDefaultConfig to scan the
// flavour dirs at all (a nil MergeFunc reads base config only). The last source wins,
// which for OKP is always the lightspeed flavour. When no enabled flavour provides the
// file (e.g. lightspeed disabled), sources is empty and no object is returned, so the
// resource is not applied.
func mergeOkpObject(sources []configSource, scheme runtime.Scheme, _ string) ([]client.Object, error) {
	if len(sources) == 0 {
		return []client.Object{}, nil
	}
	src := sources[len(sources)-1]
	objs, err := utils.ReadYamls(src.content, nil, scheme)
	if err != nil {
		return nil, fmt.Errorf("failed to parse OKP config from %s: %w", src.path, err)
	}
	return objs, nil
}
