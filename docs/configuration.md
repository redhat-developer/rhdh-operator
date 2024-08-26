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

#### app-config
As documented in [Backstage documentation](https://backstage.io/docs/conf/writing/) Backstage application is configured with one or more app-config files, merged from the first to last. Operator can contribute to the app-config list mounting ConfigMap defined in default-config/app-config.yaml as specified in the [Admin Guide](admin.md). Also, it is possible to define additional array of external, user created configMaps located in the same namespace as Backstage CR with spec.application.appConfig.

For example, we have the following ConfigMaps containing Backstage app-config configuration in the namespace:

````
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm1
data:
  "app-config1.yaml": |
    # some fragment of app-config here
  "app-config1.yaml": |
    # some fragment of app-config here
````

These app-config files can be mounted to the Backstage container as following:

````
spec:
  application:
    appConfig:
      mountPath: /my/path
      configMaps:
        - name: app-config1
        - name: app-config2
````

In this example additional app-configs are defined in the ConfigMaps called app-config1 and app-config2 and mounted as yaml files to the /my/path directory of Backstage container.

````
/my/path/app-config1.yaml
/my/path/app-config2.yaml
````

ConfigMap key/value defines file name and content and these app-configs will be applied in the same order as they declared in CR.
So, as for this example there should be 2 ConfigMaps with the content like:


created in the namespace as a prerequisite. And then Operator will create 2 files /my/path/config1.yaml and /my/path/config2.yaml and adds it to the end of backstage command line arguments as 

````
-app-config /my/path/config1.yaml -app-config /my/path/config2.yaml
````

[Includes and Dynamic Data](https://backstage.io/docs/conf/writing/#includes-and-dynamic-data) ([extra files](#extra-files) and [extra env variables](#extra-env-variables)) supported configuring extra ConfigMaps and Secrets.

#### extra files

Extra files can be mounted from pre-created ConfigMaps or Secrets. For example, we have the following objects in the namespace:

````
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm1
data:
  "file11.txt": |
    my file11 content
  "file12.txt": |
    my file12 content
````
````
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm1
data:
  "file21.txt": |
    my file1 content
  "file22.txt": |
    my file2 content
````
````
apiVersion: v1
kind: Secret
metadata:
  name: secret1
data:
  "file3.txt": |
    base64-encoded-content
````

These files can be mounted to the Backstage container as following:

````
spec:
  application:
    extraFiles:
      mountPath: /my/path
      configMaps:
        - name: cm1
        - name: cm22
          key: file21.txt
      secrets: 
        - name: secret1
          key: file3.txt
````

Operator will get either all the entries from the object if no key specified or picks specified one, create Volumes per-object and mount the files to the Backstage container.

In our example the following files will be mounted:

````
/my/path/file11.txt
/my/path/file12.txt
/my/path/file21.txt
/my/path/file3.txt
````

Note: considering possibility to limit read access to the Secrets by the Operator Service Account (by security reason), we only support mounting file from Secret if key specified.

#### extra env variables

Extra Env Variables can be injected to the Backstage container from pre-created ConfigMaps or Secrets as well as specifying name/value directly in the Custom Resource. For example, we have the following objects in the namespace:

````
apiVersion: v1
kind: ConfigMap
metadata:
  name: cm1
data:
  ENV_VAR1: "1"
  ENV_VAR2: "2"
````
````
apiVersion: v1
kind: Secret
metadata:
  name: secret1
data:
  ENV_VAR3: "base64encoded3"
  ENV_VAR4: "base64encoded4"
````

In addition, we may want to create environment variable MY_VAR=my-value declaring it directly in the Custom Resource.

````
spec:
  application:
    extraEnvs:
      configMaps:
        - name: cm1
          key: ENV_VAR1
      secrets: 
        - name: secret1
      envs:
        - name: MY_VAR
          value: "my-value"  
````

The same as extraFiles you can specify key name to inject only particular env variable. 

In our example the following env variables will be injected:

````
  ENV_VAR1 = 1
  ENV_VAR3 = 3
  ENV_VAR4 = 4
  MY_VAR = my-value
````

#### dynamic plugins



#### deployment parameters 

NOTE: these fields are deprecated as for >= 0.3.0 in favor of [spec.deployment](#deployment-configuration)

````
spec:
  application:
    image: Backstage container image:[tag], e g: "quay.io/my/my-rhdh:latest"
    replicas: number of replicas, e g: 2
    imagePullSecrets: array of image  pull secrets name, e g: \n   - my-secret-name
````

#### route



### Deployment configuration

### Local Database configuration



