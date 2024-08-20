# Configuration

It is highly recommended to read [Design](design.md) document to understand what parts Backstage consists of. 

Backstage Operator supports 3 levels: Default, Raw and Custom Resource Config.

## Default Configuration
Default Configuration defines the shape of all the Backstage instances inside Cluster. It consists of set of YAML manifests defining Kubernetes resources for Backstage instance. This configuration is located in *-default-configuration ConfigMap in a Backstage operator namespace (usually called **backstage-system** or **backstage-operator**). More details you can see in [Admin Guide](admin.md).

You can see examples of default configurations as a part of [Operator Profiles](../config/profile) in a **default-config** directory.

### Metadata generation

To make Backstage runtime consistently work some metadata values have to be predictable so the Operator generates the values following the rules below.
That means any value for those fields you may put to either Default or Raw Configuration is replaced by generated one.

- All the objects's metadata.names are generated according to the rules, defined in the [Configuration table (Object name)](admin.md)
- deployment.yaml spec.selector.matchLabels[rhdh.redhat.com/app] = backstage-<cr-name>
- deployment.yaml spec.template.metadata.labels[rhdh.redhat.com/app] = backstage-<cr-name>
- service.yaml spec.selector[rhdh.redhat.com/app] = backstage-<cr-name>
- db-statefulset.yaml spec.selector.matchLabels[rhdh.redhat.com/app] = backstage-psql-<cr-name>
- db-statefulset.yaml spec.template.metadata.labels[rhdh.redhat.com/app] = backstage-psql-<cr-name>
- db-service.yaml spec.selector[rhdh.redhat.com/app] = backstage-psql-<cr-name>

## Raw Configuration 
It is the same YAML manifests as in Default configuration but per-CR. You can override any or all Default configuration keys (e g deployment.yaml) or add not defined in Default configuration ones defining them in a ConfigMaps. 
Here's the fragment of Backstage spec containing Raw configuration
```` yaml
spec:
  rawRuntimeConfig:
    backstageConfig: <configMap-name>  # to use for all but db-*.yaml manifests 
    localDbConfig: <configMap-name>    # to use for db-*.yaml manifests
````
NOTE: As you can see, Backstage Application config is separated from Database Configuration, but, in fact, it makes no differences which ConfigMap to use for what object, finally it merged to one structure, just avoid using the same keys in both.

## Custom Resource Spec.

Desired state of resources created by Backstage Operator is defined in the Backstage Custom Resource Spec.
Here's the example of simple Backstage CR:
````
apiVersion: rhdh.redhat.com/v1alpha2
kind: Backstage
metadata:
  name: mybackstage
spec:
  application:
    appConfig:
      configMaps:
        - name: my-app-config
    extraEnvs:
      secrets:
        - name: my-secrets
````
This Custom resource defines Backstage instance called **mybackstage** and also: 
- adds additional app-config stored in **my-app-config** ConfigMap 
- adds some extra env variables stored (as a key-value pairs) in the Secret called **my-secrets**

As for API v1alpha2 (Operator v0.3.x) Backstage CR Spec contains these top-level elements:

* [application](#application-configuration)
* [deployment](#deployment-configuration)
* [database](#local-database-configuration)
* [rawRuntimeConfig](#raw-configuration)

### Application configuration

This is how Backstage Application is configured inside container. 

As documented in [Backstage doc]() Backstage application is configured with one or more app-config files, merged from the first to last. Operator can define one default OR raw app-config (as described above) AND another one (the latest in the sequence) can be defined in the Backstage CR with spec.application.appConfig



#### Custom Backstage Image

You can use the Backstage Operator to deploy a backstage application with your custom backstage image by setting the field `spec.application.image` in your Backstage CR. This is at your own risk and it is your responsibility to ensure that the image is from trusted sources, and has been tested and validated for security compliance.

### Deployment configuration

### Local Database configuration



