package v1alpha3

import (
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"
)

type BackstageConditionReason string

type BackstageConditionType string

const (
	BackstageConditionTypeDeployed BackstageConditionType = "Deployed"

	BackstageConditionReasonDeployed   BackstageConditionReason = "Deployed"
	BackstageConditionReasonFailed     BackstageConditionReason = "DeployFailed"
	BackstageConditionReasonInProgress BackstageConditionReason = "DeployInProgress"
)

// BackstageSpec defines the desired state of Backstage
type BackstageSpec struct {
	// Configuration for Backstage. Optional.
	Application *Application `json:"application,omitempty"`

	// Raw Runtime RuntimeObjects configuration. For Advanced scenarios.
	RawRuntimeConfig *RuntimeConfig `json:"rawRuntimeConfig,omitempty"`

	// Configuration for database access. Optional.
	Database *Database `json:"database,omitempty"`

	// Valid fragment of Deployment to be merged with default/raw configuration.
	// Set the Deployment's metadata and|or spec fields you want to override or add.
	// Optional.
	Deployment *BackstageDeployment `json:"deployment,omitempty"`
}

type BackstageDeployment struct {
	// Valid fragment of Deployment to be merged with default/raw configuration.
	// Set the Deployment's metadata and|or spec fields you want to override or add.
	// Optional.
	// +kubebuilder:pruning:PreserveUnknownFields
	Patch *apiextensionsv1.JSON `json:"patch,omitempty"`
}

type RuntimeConfig struct {
	// Name of ConfigMap containing Backstage runtime objects configuration
	BackstageConfigName string `json:"backstageConfig,omitempty"`
	// Name of ConfigMap containing LocalDb (PostgreSQL) runtime objects configuration
	LocalDbConfigName string `json:"localDbConfig,omitempty"`
}

type Database struct {
	// Control the creation of a local PostgreSQL DB. Set to false if using for example an external Database for Backstage.
	// +optional
	//+kubebuilder:default=true
	EnableLocalDb *bool `json:"enableLocalDb,omitempty"`

	// Name of the secret for database authentication. Optional.
	// For a local database deployment (EnableLocalDb=true), a secret will be auto generated if it does not exist.
	// The secret shall include information used for the database access.
	// An example for PostgreSQL DB access:
	// "POSTGRES_PASSWORD": "rl4s3Fh4ng3M4"
	// "POSTGRES_PORT": "5432"
	// "POSTGRES_USER": "postgres"
	// "POSTGRESQL_ADMIN_PASSWORD": "rl4s3Fh4ng3M4"
	// "POSTGRES_HOST": "backstage-psql-bs1"  # For local database, set to "backstage-psql-<CR name>".
	AuthSecretName string `json:"authSecretName,omitempty"`
}

type Application struct {
	// References to existing app-configs ConfigMap objects, that will be mounted as files in the specified mount path.
	// Each element can be a reference to any ConfigMap or Secret,
	// and will be mounted inside the main application container under a specified mount directory.
	// Additionally, each file will be passed as a `--config /mount/path/to/configmap/key` to the
	// main container args in the order of the entries defined in the AppConfigs list.
	// But bear in mind that for a single ConfigMap element containing several filenames,
	// the order in which those files will be appended to the main container args cannot be guaranteed.
	// So if you want to pass multiple app-config files, it is recommended to pass one ConfigMap per app-config file.
	// +optional
	AppConfig *AppConfig `json:"appConfig,omitempty"`

	// Reference to an existing ConfigMap for Dynamic Plugins.
	// A new one will be generated with the default config if not set.
	// The ConfigMap object must have an existing key named: 'dynamic-plugins.yaml'.
	// +optional
	DynamicPluginsConfigMapName string `json:"dynamicPluginsConfigMapName,omitempty"`

	// References to existing Config objects to use as extra config files.
	// They will be mounted as files in the specified mount path.
	// Each element can be a reference to any ConfigMap or Secret.
	// +optional
	ExtraFiles *ExtraFiles `json:"extraFiles,omitempty"`

	// Extra environment variables
	// +optional
	ExtraEnvs *ExtraEnvs `json:"extraEnvs,omitempty"`

	// Number of desired replicas to set in the Backstage Deployment.
	// Defaults to 1.
	// +optional
	//+kubebuilder:default=1
	Replicas *int32 `json:"replicas,omitempty"`

	// Custom image to use in all containers (including Init Containers).
	// It is your responsibility to make sure the image is from trusted sources and has been validated for security compliance
	// +optional
	Image *string `json:"image,omitempty"`

	// Image Pull Secrets to use in all containers (including Init Containers)
	// +optional
	ImagePullSecrets []string `json:"imagePullSecrets,omitempty"`

	// Route configuration. Used for OpenShift only.
	Route *Route `json:"route,omitempty"`
}

type AppConfig struct {
	// Mount path for all app-config files listed in the ConfigMapRefs field
	// +optional
	// +kubebuilder:default=/opt/app-root/src
	MountPath string `json:"mountPath,omitempty"`

	// List of ConfigMaps storing the app-config files. Will be mounted as files under the MountPath specified.
	// For each item in this array, if a key is not specified, it means that all keys in the ConfigMap will be mounted as files.
	// Otherwise, only the specified key will be mounted as a file.
	// Bear in mind not to put sensitive data in those ConfigMaps. Instead, your app-config content can reference
	// environment variables (which you can set with the ExtraEnvs field) and/or include extra files (see the ExtraFiles field).
	// More details on https://backstage.io/docs/conf/writing/.
	// +optional
	ConfigMaps []FileObjectRef `json:"configMaps,omitempty"`
}

type ExtraFiles struct {
	// Mount path for all extra configuration files listed in the Items field
	// +optional
	// +kubebuilder:default=/opt/app-root/src
	MountPath string `json:"mountPath,omitempty"`

	// List of references to ConfigMaps objects mounted as extra files under the MountPath specified.
	// For each item in this array, if a key is not specified, it means that all keys in the ConfigMap will be mounted as files.
	// Otherwise, only the specified key will be mounted as a file.
	// +optional
	ConfigMaps []FileObjectRef `json:"configMaps,omitempty"`

	// List of references to Secrets objects mounted as extra files under the MountPath specified.
	// For each item in this array, a key must be specified that will be mounted as a file.
	// +optional
	Secrets []FileObjectRef `json:"secrets,omitempty"`

	// List of references to Persistent Volume Claim objects mounted as extra files
	// For each item in this array, a key must be specified that will be mounted as a file.
	// +optional
	Pvcs []PvcRef `json:"pvcs,omitempty"`
}

type ExtraEnvs struct {
	// List of references to ConfigMaps objects to inject as additional environment variables.
	// For each item in this array, if a key is not specified, it means that all keys in the ConfigMap will be injected as additional environment variables.
	// Otherwise, only the specified key will be injected as an additional environment variable.
	// +optional
	ConfigMaps []EnvObjectRef `json:"configMaps,omitempty"`

	// List of references to Secrets objects to inject as additional environment variables.
	// For each item in this array, if a key is not specified, it means that all keys in the Secret will be injected as additional environment variables.
	// Otherwise, only the specified key will be injected as environment variable.
	// +optional
	Secrets []EnvObjectRef `json:"secrets,omitempty"`

	// List of name and value pairs to add as environment variables.
	// +optional
	Envs []Env `json:"envs,omitempty"`
}

type EnvObjectRef struct {
	// Name of the object
	// We support only ConfigMaps and Secrets.
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	// Key in the object
	// +optional
	Key string `json:"key,omitempty"`
}

type FileObjectRef struct {
	// Name of the object
	// Supported ConfigMaps and Secrets
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	// Key in the object
	// +optional
	Key string `json:"key,omitempty"`

	// Path to mount the Object. If not specified default-path/Name will be used
	// +optional
	MountPath string `json:"mountPath"`
}

type PvcRef struct {
	// Name of the object
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	// Path to mount PVC. If not specified default-path/Name will be used
	// +optional
	MountPath string `json:"mountPath"`
}

type Env struct {
	// Name of the environment variable
	//+kubebuilder:validation:Required
	Name string `json:"name"`

	// Value of the environment variable
	//+kubebuilder:validation:Required
	Value string `json:"value"`
}

// BackstageStatus defines the observed state of Backstage
type BackstageStatus struct {
	// Conditions is the list of conditions describing the state of the runtime
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

//+kubebuilder:object:root=true
//+kubebuilder:subresource:status
//+kubebuilder:storageversion

// Backstage is the Schema for the backstages API
type Backstage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackstageSpec   `json:"spec,omitempty"`
	Status BackstageStatus `json:"status,omitempty"`
}

//+kubebuilder:object:root=true

// BackstageList contains a list of Backstage
type BackstageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Backstage `json:"items"`
}

// Route specifies configuration parameters for OpenShift Route for Backstage.
// Only a secured edge route is supported for Backstage.
type Route struct {
	// Control the creation of a Route on OpenShift.
	// +optional
	//+kubebuilder:default=true
	Enabled *bool `json:"enabled,omitempty"`

	// Host is an alias/DNS that points to the service. Optional.
	// Ignored if Enabled is false.
	// If not specified a route name will typically be automatically
	// chosen.  Must follow DNS952 subdomain conventions.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`
	Host string `json:"host,omitempty" protobuf:"bytes,1,opt,name=host"`

	// Subdomain is a DNS subdomain that is requested within the ingress controller's
	// domain (as a subdomain).
	// Ignored if Enabled is false.
	// Example: subdomain `frontend` automatically receives the router subdomain
	// `apps.mycluster.com` to have a full hostname `frontend.apps.mycluster.com`.
	// +optional
	// +kubebuilder:validation:MaxLength=253
	// +kubebuilder:validation:Pattern=`^([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9])(\.([a-zA-Z0-9]|[a-zA-Z0-9][a-zA-Z0-9\-]{0,61}[a-zA-Z0-9]))*$`
	Subdomain string `json:"subdomain,omitempty"`

	// The tls field provides the ability to configure certificates for the route.
	// Ignored if Enabled is false.
	// +optional
	TLS *TLS `json:"tls,omitempty"`
}

type TLS struct {
	// certificate provides certificate contents. This should be a single serving certificate, not a certificate
	// chain. Do not include a CA certificate.
	Certificate string `json:"certificate,omitempty"`

	// ExternalCertificateSecretName provides certificate contents as a secret reference.
	// This should be a single serving certificate, not a certificate
	// chain. Do not include a CA certificate. The secret referenced should
	// be present in the same namespace as that of the Route.
	// Forbidden when `certificate` is set.
	// Note that securing Routes with external certificates in TLS secrets is a Technology Preview feature in OpenShift,
	// and requires enabling the `RouteExternalCertificate` OpenShift Feature Gate and might not be functionally complete.
	// +optional
	ExternalCertificateSecretName string `json:"externalCertificateSecretName,omitempty"`

	// key provides key file contents
	Key string `json:"key,omitempty"`

	// caCertificate provides the cert authority certificate contents
	CACertificate string `json:"caCertificate,omitempty"`
}

func init() {
	SchemeBuilder.Register(&Backstage{}, &BackstageList{})
}

// IsLocalDbEnabled returns true if Local database is configured and enabled
func (s *BackstageSpec) IsLocalDbEnabled() bool {
	if s.Database == nil {
		return true
	}
	return ptr.Deref(s.Database.EnableLocalDb, true)
}

// IsRouteEnabled returns value of Application.Route.Enabled if defined or true by default
func (s *BackstageSpec) IsRouteEnabled() bool {
	if s.Application != nil && s.Application.Route != nil {
		return ptr.Deref(s.Application.Route.Enabled, true)
	}
	return true
}

func (s *BackstageSpec) IsAuthSecretSpecified() bool {
	return s.Database != nil && s.Database.AuthSecretName != ""
}
