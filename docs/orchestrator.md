## Orchestrator installation and configuration

### Prerequisites for Orchestrator Plugin Installation on OpenShift

To install the Orchestrator plugin on OpenShift, the following components are required:

- **OpenShift Serverless Operator**
- **Knative Serving**
- **Knative Eventing**
- **OpenShift Serverless Logic Operator**


### Methods to Install required infrastructure

There are 3 methods to install the required components for the Orchestrator plugin on OpenShift:
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
   bash plugin-infra.sh
   ```  

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
Ensure to add a secret with the BACKEND_SECRET key/value and update
the secret name in the `Backstage` CR under the `extraEnvs` field.

#### Plugin registry

As for RHDH 1.7 the orchestrator plugin packages are located in **npm.registry.redhat.com** NPM registry, which is preconfigured in rhdh default-config.

#### Plugin dependencies

The orchestrator plugin instance requires the following dependencies to be installed:
- A SonataflowPlatform custom resource - created in the namespace of the Backstage CR.
- A set of NetworkPolicies to allow traffic between infra resources (knative and serverless logic operator) created in the namespace of Backstage CR, traffic for monitoring, and intra-namespace traffic.

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

**Note**: Currently, RHDH Orchestrator workflow is configured and setup to run within the same namespace as RHDH instance (CR).
However, to enable and configure the deployment of workflows in a separated namespace, please follow the steps in this [section](#optional-enabling-workflow-in-a-different-namespace).
##### RBAC

To enable the Backstage operator to work with the SonataFlow platform, its ServiceAccount must be granted the appropriate permissions. 
This is done by the Backstage operator automatically when the SonataFlowPlatform CR is created in the namespace of the Backstage CR by the appropriate Role and RoleBinding resource in the [profile/rhdh/plugin-rbac directory](../config/profile/rhdh/plugin-rbac).



## Optional: Enabling workflow in a different namespace
To enable workflow deployment in another namespace other than where RHDH Orchestrator infrastructure are deployed and configured,
please follow these steps below.

**Note**: The `$RHDH_NAMESPACE` is the namespace where RHDH instance (CR) is deployed.
Please ensure to update this value as needed.

1. Add `SonataFlowClusterPlatform` Custom Resource: 
  ```console
  oc create -f - <<EOF
  apiVersion: sonataflow.org/v1alpha08
  kind: SonataFlowClusterPlatform
  metadata:
    name: cluster-platform
  spec:
    platformRef:
      name: sonataflow-platform
      namespace: $RHDH_NAMESPACE
  EOF
   ```

2. Add Network Policies: To allow communication between RHDH namespace and the workflow namespace, 
two network policies need to be added.

   ###### Allow Traffic from the Workflow Namespace:
   To allow RHDH services to accept traffic from workflows, create an additional network policy within
   the RHDH instance namespace.

   ```console
   oc create -f - <<EOF
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-external-workflows-to-rhdh
     # Namespace where network policies are deployed
     namespace: $RHDH_NAMESPACE
   spec:
     podSelector: {}
     ingress:
       - from:
         - namespaceSelector:
             matchLabels:
               # Allow SonataFlow services to communicate with new/additional workflow namespace.
               kubernetes.io/metadata.name: $ADDITIONAL_WORKFLOW_NAMESPACE
   EOF
   ```
   ###### Allow traffic from RHDH and Knative namespaces:
   To allow traffic from RHDH, SonataFlow and Knative, create a network policy within the new/additional workflow namespace.

   ```console
   oc create -f - <<EOF
   apiVersion: networking.k8s.io/v1
   kind: NetworkPolicy
   metadata:
     name: allow-rhdh-and-knative-to-workflows
     namespace: $ADDITIONAL_WORKFLOW_NAMESPACE
   spec:
     podSelector: {}
     ingress:
       - from:
         - namespaceSelector:
             matchLabels:
               # Allows traffic from pods in the RHDH namespace.
               kubernetes.io/metadata.name: $RHDH_NAMESPACE
         - namespaceSelector:
             matchLabels:
               # Allows traffic from pods in the Knative Eventing namespace.
               kubernetes.io/metadata.name: knative-eventing
         - namespaceSelector:
             matchLabels:
               # Allows traffic from pods in the Knative Serving namespace.
               kubernetes.io/metadata.name: knative-serving
   EOF
   ```



