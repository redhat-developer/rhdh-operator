## Orchestrator installation and configuration

### Prerequisites for Orchestrator Plugin Installation on OpenShift

To install the Orchestrator plugin on OpenShift, the following components are required:

- **OpenShift Serverless Operator**
- **Knative Serving**
- **Knative Eventing**
- **OpenShift Serverless Logic Operator**
Optionally:
- **OpenShift Pipelines Operator (Tekton)**
- **OpenShift GitOps Operator (ArgoCD)**


### Methods to Install required infrastructure

There are three methods to install the required components for the Orchestrator plugin on OpenShift:
- [Manual Installation](#manual-installation)
- [RHDH helper script](#rhdh-helper-script)
- [RHDH Orchestrator Infra Helm Chart](#rhdh-orchestrator-infra-helm-chart)

#### Manual Installation
This method is recommended for production environments, especially when specific versions of the required components are installed (e.g., when OpenShift Serverless is already in use by other applications).

##### Version Compatibility
This recommendation is based on OpenShift Serverless version `1.36`. Refer to the Orchestrator plugin compatibility documentation to ensure the correct version of OpenShift Serverless is supported for your Orchestrator plugin version.

##### Steps
Go to the [OpenShift Serverless documentation](https://docs.redhat.com/en/documentation/red_hat_openshift_serverless) for installation instructions and follow these steps:
1. Preparing to install OpenShift Serverless.
2. Installing the OpenShift Serverless Operator.
3. Installing Knative Serving.
4. Installing Knative Eventing.
5. Installing the OpenShift Serverless Logic Operator.
Optionally:
6. Installing the [OpenShift Pipelines Operator](https://docs.redhat.com/en/documentation/red_hat_openshift_pipelines) (Tekton)
7. Installing the [OpenShift GitOps Operator](https://docs.redhat.com/en/documentation/red_hat_openshift_gitops) (ArgoCD)


#### RHDH helper script
This script provides a quick way to install the OpenShift Serverless infrastructure for the Orchestrator plugin. It is safe to use in empty clusters but should be used with caution in production clusters.
**Note:** Current Subscriptions configuration uses **Automatic** install plan (**spec.installPlanApproval: Automatic**), consider to change it to **Manual** if you want to control the installation of the operators (see [Operator Installation with OLM](https://olm.operatorframework.io/docs/tasks/install-operator-with-olm) for more details).

##### Steps
1. Download the `plugin-infra.sh` script:
   ```bash
   curl -sSLO https://raw.githubusercontent.com/redhat-developer/rhdh-operator/refs/heads/release-1.7/config/profile/rhdh/plugin-infra/plugin-infra.sh
   ```  
You can specify the RHDH version in the URL (`/release-X.Y/`, e.g., `1.7` in this example) or use main.
2. Run the script:
   ```bash
   bash plugin-infra.sh [--with-cicd] [delete] [--branch <branch>]
   ```  
Flags:
* `--with-cicd` flag will also install the OpenShift Pipelines Operator (Tekton) and OpenShift GitOps Operator (ArgoCD) in addition to the required components for the Orchestrator plugin. To continue the configuration for CICD, please follow this [guide](orchestrator-cicd.md).
* `delete`  will delete the installed components instead of installing them.
* `--branch <branch>` flag allows to specify the branch of the RHDH Operator repository where the configuration yaml files will be taken (ignored if you have local yaml files). If not specified, it defaults to the `main` branch.

The script is checking if the directory where plugin-infra.sh is located contains the corresponding configuration files, e.g., serverless.yaml, knative.yaml, serverless-logic.yaml... (see [plugin-infra directory](../config/profile/rhdh/plugin-infra) for the complete list). If the files are not found, it will download them from the specified branch of the RHDH Operator repository.

#### RHDH Orchestrator Infra Helm Chart
This method has similar usage and cautions as the RHDH Helper Utility.

##### Steps
1. Install the required components using the Orchestrator Infra Helm chart.*(TODO: Replace with downstream chart if applicable.)*

### Installing the Orchestrator Plugin

The orchestrator plugin (as of v1.6.0) consists of four dynamic plugins:
- orchestrator-backend
- orchestrator-frontend
- orchestrator-scaffolder-backend-module
- orchestrator-form-widgets

As for RHDH 1.7 all of these plugins are included in the default dynamic-plugins.yaml file of **install-dynamic-plugins** container but disabled by default.
To enable the orchestrator plugin, you should refer the dynamic plugins ConfigMap with following data in your Backstage Custom Resource (CR):
```yaml
    includes:
      - dynamic-plugins.default.yaml
    plugins:
       - package: "@redhat/backstage-plugin-orchestrator@1.6.0"
         disabled: false
       - package: "@redhat/backstage-plugin-orchestrator-backend-dynamic@1.6.0"
         disabled: false
         dependencies:
            - ref: sonataflow
       - package: "@redhat/backstage-plugin-scaffolder-backend-module-orchestrator-dynamic@1.6.0"
         disabled: false
       - package: "@redhat/backstage-plugin-orchestrator-form-widgets@1.6.0"
         disabled: false  
```

See [example](/examples/orchestrator.yaml) for a complete configuration of the orchestrator plugin.

#### Plugin registry

As for RHDH 1.7 the orchestrator plugin packages are located in **npm.registry.redhat.com** NPM registry, which is preconfigured in rhdh default-config.

#### Plugin dependencies

The orchestrator plugin instance requires the following dependencies to be installed:
- A SonataflowPlatform custom resource - created in the namespace of the Backstage CR.
- A set of NetworkPolicies to allow traffic between Knative resources created in the namespace of Backstage CR, traffic for monitoring, and intra-namespace traffic.

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

Where `{{backstage-name}}` is the name of the Backstage Custom Resource (CR) and `{{backstage-ns}}` is the namespace where the Backstage CR is created.

Current **default** implementation of the orchestrator plugin dependencies uses:
- the PostgreSQL database created by Backstage for Orchestrator plugin, named **backstage_plugin_orchestrator**
- the Secret created by Backstage operator for the PostgreSQL with **POSTGRES_USER** and **POSTGRES_PASSWORD** keys as the database credentials in the Backstage CR namespace.
- the Service created by Backstage operator for the PostgreSQL database with the name **backstage-psql-{{backstage-name}}** in the Backstage CR namespace.

See [profile/rhdh/plugin-deps](/config/profile/rhdh/plugin-deps) for a complete configuration of the orchestrator plugin dependencies.

##### RBAC

To enable the Backstage operator to work with the SonataFlow platform, its ServiceAccount must be granted the appropriate permissions. 
This is done by the Backstage operator automatically when the SonataFlowPlatform CR is created in the namespace of the Backstage CR by the appropriate Role and RoleBinding resource in the [profile/rhdh/plugin-rbac directory](../config/profile/rhdh/plugin-rbac).

## Optional: Enable GitOps/Pipelines for Orchestrator Workflows
To enable CI/CD for RHDH Orchestrator workflows, please follow this [guide](orchestrator-cicd.md).



