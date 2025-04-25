package platform

// Platform represents the Kubernetes platform type
type Platform struct {
	Name      string
	Extension string
}

var (
	OpenShift  = Platform{Name: "OpenShift", Extension: "ocp"}
	EKS        = Platform{Name: "EKS", Extension: "k8s"}
	AKS        = Platform{Name: "AKS", Extension: "k8s"}
	GKE        = Platform{Name: "GKE", Extension: "k8s"}
	Kubernetes = Platform{Name: "Kubernetes", Extension: "k8s"}
	Default    = Kubernetes
)

func (p Platform) IsOpenshift() bool {
	return p == OpenShift
}
