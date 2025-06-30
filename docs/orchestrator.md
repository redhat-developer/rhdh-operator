## Orchestrator installation and configuration

### Prerequisites for Orchestrator Plugin Installation on OpenShift

To install the Orchestrator plugin on OpenShift, the following components are required:

- **OpenShift Serverless Operator**
- **Knative Serving**
- **Knative Eventing**
- **OpenShift Serverless Logic Operator**

---

### Methods to Install Required infrastructure

#### 1. **Manual Installation**
This method is recommended for production environments, especially when specific versions of the required components are installed (e.g., when OpenShift Serverless is already in use by other applications).

##### Version Compatibility
This recommendation is based on OpenShift Serverless version `1.36`. Refer to the Orchestrator plugin compatibility documentation to ensure the correct version of OpenShift Serverless is supported for your Orchestrator plugin version.

##### Steps
1. [Prepare your infrastructure for OpenShift Serverless installation.](https://docs.redhat.com/en/documentation/red_hat_openshift_serverless/1.36/html/installing_openshift_serverless/preparing-serverless-install)
2. [Install the OpenShift Serverless Operator.](https://docs.redhat.com/en/documentation/red_hat_openshift_serverless/1.33/html/installing_openshift_serverless/preparing-serverless-install)
3. [Install Knative Serving.](https://docs.redhat.com/en/documentation/red_hat_openshift_serverless/1.36/html/installing_openshift_serverless/installing-knative-serving)
4. [Install Knative Eventing.](https://docs.redhat.com/en/documentation/red_hat_openshift_serverless/1.36/html/installing_openshift_serverless/installing-knative-eventing)
5. [Install the OpenShift Serverless Logic Operator.](https://docs.redhat.com/en/documentation/red_hat_openshift_serverless/1.36/html/installing_openshift_serverless/install-serverless-logic-operator)

---

#### 2. **RHDH Helper script**
This script provides a quick way to install the OpenShift Serverless infrastructure for the Orchestrator plugin. It is safe to use in empty clusters but should be used with caution in production clusters.

##### Steps
1. Download the `plugin-infra.sh` script:
   ```bash
   curl -sSLO https://raw.githubusercontent.com/redhat-developer/rhdh-operator/refs/heads/release-1.7/config/profile/rhdh/plugin-infra/plugin-infra.sh
   ```  
You can specify the RHDH version in the URL (`/release-X.Y/`, e.g., `1.7` in this example) or main .
2. Run the script:
   ```bash
   bash plugin-infra.sh
   ```  

---

#### 3. **RHDH Orchestrator Infra Helm Chart**
This method has similar usage and cautions as the RHDH Helper Utility.

##### Steps
1. Install the required components using the Orchestrator Infra Helm chart.*(TODO: Replace with downstream chart if applicable.)*

### Installing the Orchestrator Plugin

The orchestrator plugin (as of v1.5.1) consists of three dynamic plugins:
- orchestrator-backend
- orchestrator-frontend
- orchestrator-scaffolder-backend-module

As for RHDH 1.7 all of these plugins are included in the default dynamic-plugins.yaml file of **install-dynamic-plugins** container but disabled by default.
To enable the orchestrator plugin, you should refer the dynamic plugins ConfigMap with following data in your Backstage Custom Resource (CR):
```yaml
    includes:
      - dynamic-plugins.default.yaml
    plugins:
      - disabled: false
        package: "@redhat/backstage-plugin-orchestrator@1.5.1"
      - disabled: false
        package: "@redhat/backstage-plugin-orchestrator-backend-dynamic@1.5.1"
        dependencies:
          - ref: sonataflow
      - disabled: false
        package: "@redhat/backstage-plugin-orchestrator-backend-dynamic@1.5.1"
```

See [example](/examples/orchestrator.yaml) for a complete configuration of the orchestrator plugin.

#### Plugin registry

As for RHDH 1.7 the orchestrator plugin packages are located in **npm.registry.redhat.com** NPM registry, which is preconfigured in rhdh default-config.

#### Plugin dependencies

The orchestrator plugin instance requires the following dependencies to be installed:
- A Sonataflowplatform custom resource - created in the namespace of the Backstage CR.
- A set of NetworkPolicies to allow traffic between Knative resources created in the namespace of Backstage CR, traffic for monitoring, and intra-namespace traffic.

Both of them are configured in the `plugin-deps/sonataflow.yaml` (See [profile/rhdh/plugin-deps](/config/profile/rhdh/plugin-deps) ) file of the profile and referenced in the dynamic-plugins.yaml file of the Backstage CR as dependencies of **backstage-plugin-orchestrator-backend-dynamic** plugin.




