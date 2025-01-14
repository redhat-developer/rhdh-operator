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

As of January 2025, there are three predefined profiles:

* **rhdh**: The default profile, applied if no explicit PROFILE is specified. This profile contains configurations for the Red Hat Developer Hub.
* **backstage.io**: A simple configuration for a bare Backstage instance, utilizing the image available at https://github.com/backstage/backstage/pkgs/container/backstage.
* **external**: A basis for third-party configurations external to the Backstage repository.

## Creating a New Profile
User may want to create a new Configuration Profile for a specific use case, such as:
* A custom Backstage Default Configuration by providing a specific default-config directory
* A specific configuration for the Operator controller's deployment by providing patches for the base deployment manifest
* A specific name, labels, or annotations for the Operator namespace by providing a specific namespace manifest
* A specific template for ClusterServiceVersion (CSV) manifests by providing a specific CSV manifest in the config/manifests directory

To create a new Configuration Profile and make it available for test, integration test, and deployment, create a directory with the profile name under the **./config/profile** directory. The directory should contain the following files:
* **kustomization.yaml**: A Kustomize file defining the resources. See [config/profile/rhdh/kustomization.yaml](RHDH profile) for an example.
* **default-config**: A directory containing the Operator Default Configuration. See the [Default Configuration](configuration.md#default-configuration) section for more information.
* **namespace.yaml**: A Kubernetes manifest file defining the namespace for the Operator.
* Optionally **patches**: A directory containing patches for the Operator deployment.

To add a custom ClusterServiceVersion (CSV) manifest, create a directory with the profile name under the **./config/manifests** directory. The directory should contain the following files:
* **kustomization.yaml**: A Kustomize file defining the resources. See [config/manifests/rhdh/kustomization.yaml](RHDH manifests) for an example.
* **bases/csv.yaml**: A Kubernetes manifest file defining the ClusterServiceVersion.

### External Profiles
To create a Configuration Profile external to the Backstage Operator repository, create a directory following the same structure as above, and reference the **external** profile in **kustomization.yaml** through the repository URL. Here is an example:

```yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: backstage-system

resources:
  - https://github.com/redhat-developer/rhdh-operator/config/profile/external
  - namespace.yaml

namePrefix: backstage-

generatorOptions:
  disableNameSuffixHash: true
configMapGenerator:
  - files:
      - default-config/deployment.yaml
      - default-config/service.yaml
      - default-config/app-config.yaml
    name: default-config
```
See more about how to [reference remote target in Kustomize](https://github.com/kubernetes-sigs/kustomize/blob/master/examples/remoteBuild.md).

To deploy the Operator with the external profile, you can use the following command:

```bash
kusomize build . | kubectl apply -f -
```
assuming you have installed Kustomize and run the command from the kustomization.yaml's directory.

## Platform patches

Certain Profiles may need additional patches to the Operator default configuration. For example, the Red Hat Developer Hub (RHDH) Profile requires additional patches to the Operator deployment manifest to work correctly with container's filesystem on vanilla Kubernetes vs Openshift.
For this purpose, the Operator provides a way to apply additional patches to the Profile's default configuration. These patches are provided as a YAML files with the same name as the default configuration file, but with the **.{PlatformID}** extension.
For the time being (v0.5), there are two PlatformIDs: **k8s** for vanilla Kubernetes and **ocp** for Openshift platform.

Patch application is done by the Operator during the deployment process as following:
1. The Operator reads and applies the default configuration file from the Profile's directory (e g deployment.yaml).
2. The Operator recognizes current platform and checks if there is a patch file with the same name as the default configuration file, but with the **.{PlatformID}** extension (e g deployment.yaml.k8s).
3. If the patch file is found, the Operator reads and applies it to the default configuration.

**Note:** For the most of the cases, when your organization has a specific platform or environment requirements, you may NOT need to provide additional patches to the Operator deployment. 
