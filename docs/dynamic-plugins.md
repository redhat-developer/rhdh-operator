## Dynamic plugins dependency management

### Overview
Dynamic plugins configured for the Backstage CR may require certain Kubernetes resources to be configured to make the plugin work. These are referred to as 'plugin dependencies'. Starting from version 1.7, it is possible to automatically create these resources when the Backstage CR is applied to the cluster.

### Profile Configuration
Plugin dependency configuration for a specific profile is done via the `/config/profile/{PROFILE}/plugin-deps` directory. To enable this, the administrator should place the required resources as Kubernetes manifests in YAML format within **plugin-deps** directory.

**Example Directory Structure**:
```txt
config/
  profile/
    rhdh/
     kustomization.yaml
     plugin-deps/
        example-dep1.yaml
        example-dep2.yaml
```
Here, **example-dep1.yaml** and **example-dep2.yaml** are the plugin dependencies for the example plugin.

**Notes:**  

* If a resource manifest does not specify a namespace, it will be created in the namespace of the Backstage CR.
* Resources may contain **{{backstage-name}}** and **{{backstage-ns}}** placeholders, which will be replaced with the name and namespace of the Backstage CR, respectively.

The `kustomization.yaml` file should contain the following lines:
```yaml

configMapGenerator:
  - files:
      - plugin-deps/example-dep1.yaml
      - plugin-deps/example-dep2.yaml
    name: plugin-deps
```

### Plugin dependencies infrastructure

If plugin dependencies require infrastructural resources (e.g. other Operators to be installed) and if the User (Administrator) wants it to be deployed (see Note below), they can be specified in the /config/profile/{PROFILE}/plugin-infra directory. To create these resources (along with the operator deployment), use the `make plugin-infra` command. 

**Note**: Be cautious when running this command on a production cluster, as it may reconfigure cluster-scoped resources.

### Plugin configuration

To create the plugin dependencies when the Backstage CR is applied, they must be referenced in the **dependencies** field of the plugin configuration. The operator will look for the **plugin-deps** directory and create the resources described in the files within this directory.  

Plugin dependencies can be referenced in the dynamic-plugins' ConfigMap. This can either be part of the profile's [default configuration](configuration.md/#default-configuration-files) for all Backstage CRs or part of the [ConfigMap referenced in the Backstage CR](configuration.md/#dynamic-plugins). Starting from version 1.7, plugin dependencies can be included in the dynamic plugin configuration. Each `dependencies.ref` value can either match the full file name or serve as a prefix for the file name. The operator will look for files in the `plugin-deps` directory whose names either start with the specified `ref` value or exactly match it. These files will be used to create the resources described within them. 

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: default-dynamic-plugins
data:
  dynamic-plugins.yaml: |
    includes:
      - dynamic-plugins.default.yaml
    plugins:
      - disabled: false
        package: "path-or-url-to-example-plugin"
        dependencies:
          - ref: example-dep
```

In this example, both example-dep1.yaml and example-dep1.yaml will be picked and operator create the resources described in the files. 

### Example: Orchestrator plugin dependencies
The orchestrator plugin (as of v1.5.1) consists of three dynamic plugins:
- orchestrator-backend
- orchestrator-frontend
- scaffolder-backend-module
See [example](/examples/orchestrator.yaml) for a complete configuration of the orchestrator plugin.


The orchestrator plugin has the following dependencies:
- A Sonataflowplatform custom resource - created in the namespace of the Backstage CR.
- Knativeeventing and Knativeserving custom resources to be created in the knative-eventing and knative-serving namespaces respectively.
- A set of NetworkPolicies to allow traffic between Knative resources created in the namespace of Backstage CR, traffic for monitoring, and intra-namespace traffic.
See [profile/rhdh/plugin-deps](/config/profile/rhdh/plugin-deps)

The orchestrator-backend plugin uses the service **sonataflow-platform-data-index-service**, which is created by the SonataFlowPlatform CR. This service is used to communicate with the SonataFlow platform.

**Important:** The sonataflowplatform CR contains dataIndex service that requires PostgreSQL database. 

```yaml
      persistence:
        postgresql:
          secretRef:
            name: backstage-psql-secret-{{backstage-name}}
            userKey: POSTGRES_USER
            passwordKey: POSTGRES_PASSWORD
          serviceRef:
            name: backstage-psql-{{backstage-name}}
            namespace: {{backstage-ns}}
            databaseName: backstage_plugin_orchestrator
```

Current **default** implementation of the orchestrator plugin dependencies uses:
- the PostgreSQL database created by Backstage for Orchestrator plugin, named **backstage_plugin_orchestrator** 
- the Secret created by Backstage operator for the PostgreSQL with **POSTGRES_USER** and **POSTGRES_PASSWORD** keys as the database credentials in the Backstage CR namespace. 
- the Service created by Backstage operator for the PostgreSQL database with the name **backstage-psql-{{backstage-name}}** in the Backstage CR namespace.

**Note:** This default implementation is not recommended for production. For production environments, you should configure an external database connection instead (**TODO: Provide an example with an external database**).

**Known issue:** 
Since DB secret with credentials is automatically generated by the Operator every time new Backstage CR created and sonataflowplatform is not recreated and so uses the "old" secret, the orchestrator plugin will not work properly. To resolve this, the user must manually recreate the SonataFlowPlatform CR or its pods.

The SonataFlowPlatform CR requires the SonataFlow operator to be installed. In an OpenShift environment, it is done by installing the OpenShift Serverless Operator and the OpenShift Serverless Logic Operator by installing respective OLM Subscriptions (see [infra-serverless.yaml](/config/profile/rhdh/plugin-infra/orchestrator/infra-serverless.yaml) and [infra-sonataflow.yaml](/config/profile/rhdh/plugin-infra/orchestrator/infra-sonataflow.yaml)).

**Note:** Current Subscriptions configuration uses **Automatic** install plan (**spec.installPlanApproval: Automatic**), consider to change it to **Manual** if you want to control the installation of the operators (see [Operator Installation with OLM](https://olm.operatorframework.io/docs/tasks/install-operator-with-olm) for more details).

**Note:** Using plugin infrastructure (plugin-infra) can be risky in production, as it modifies cluster-scoped resources.

#### RBAC

Additionally, to enable the Backstage operator to work with the SonataFlow platform, its ServiceAccount must be granted the appropriate permissions. 

These manifests are defined in the profile/rhdh/operator/plugin-rbac/rbac-sonataflow.yaml file which is applied along with the Backstage operator deployment.
