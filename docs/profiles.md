# Configuration Profiles

Starting with version 0.4.0, the Backstage Operator supports the concept of **Configuration Profiles**.

A Configuration Profile is a predefined and named Backstage Operator [Default Configuration](configuration.md#default-configuration), along with additional metadata for OLM manifests to be applied at deploy time (when deploying the Operator on a Kubernetes cluster). This approach allows us to decouple the Operator Controller (which provides functionality through binaries) from the set and shape of Kubernetes resources created and managed by the Operator.

## How It Works

Like the majority of Kubernetes Operator SDK-based operators, the Backstage Operator leverages Make and Kustomize to build, test, and construct the necessary Kubernetes manifests for deployment to the cluster, either directly or through OLM. Adding **PROFILE=<profile-name>** to any of the Make commands listed below, or to commands using these targets, will utilize one of the predefined Configuration Profiles.

### Affected Make Targets

* **test**
* **integration-test**
* **run**
* **deploy/undeploy**
* **deployment-manifest**
* **bundle**
* **bundle-build**

For more information on Make commands, refer to the [Developer Guide](developer.md).

For example:

```
make deploy PROFILE=rhdh
```

This command will deploy the Operator on the current cluster using the **rhdh** profile.

## Profile Definition

A Configuration Profile consists of a directory with a specific structure named after the profile. There are two locations to define it:

* **./config/profile/<profile-name>**: Contains the Operator Default Configuration. Example: [./config/profile/rhdh](../config/profile/rhdh)
* **./config/manifests/<profile-name>**: Contains the `kustomization.yaml` file and base configuration for creating the OLM bundle, as described in the **make bundle** command.

## Out-of-the-box Configuration Profiles

As of September 2024, there are two predefined profiles:

* **rhdh**: The default profile, applied if no explicit PROFILE is specified. This profile contains configurations for the Red Hat Developer Hub.
* **backstage.io**: A simple configuration for a bare Backstage instance, utilizing the image available at https://github.com/backstage/backstage/pkgs/container/backstage.

Additionally, there is a third profile, currently a work in progress (TBD), called "external," which is intended to be used as a template for third-party configurations external to the Backstage repository. This serves mostly as a placeholder for the time being.
