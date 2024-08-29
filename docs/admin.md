# Administrator Guide

This guide is intended for the Backstage Operator administrator who:

* Possesses sufficient knowledge and rights to configure Kubernetes clusters and cluster-scoped objects.
* Has acquired enough understanding to configure and support the Backstage Operator (with the assistance of this document).
* Is not necessarily an expert in Backstage functionality and configuration.

## Default Backstage instance configuration

The Backstage Operator operates at the cluster level, enabling management of multiple Backstage instances (Custom Resources).

Each Backstage Custom Resource (CR) governs the creation, modification, and deletion of a set of Kubernetes objects.

The default shape of these objects is configured at the Operator level using YAML files containing Kubernetes manifests.

Default Configuration is implemented as a ConfigMap named `backstage-default-config`, deployed within the Kubernetes namespace where the operator is installed (usually `backstage-system` or `rhdh-operator`). This ConfigMap is mounted to the `/default-config` directory of the Backstage controller container.

See [Configuration](configuration.md) -> Default Configuration for more details.

### Operator Bundle configuration 

With Backstage Operator's Makefile you can generate bundle descriptor using *make bundle* command

Along with CSV manifest it generates default-config ConfigMap manifest, which can be modified and applied to Backstage Operator.

[//]: # (TODO: document how an administrator can make changes to the default operator configuration, using their own configuration file (perhaps based on the generated one), and apply it using `kubectl` or `oc`.

### Kustomize deploy configuration

Make sure use the current context in your kubeconfig file is pointed to correct place, change necessary part of your config/manager/default-config or just replace some of the file(s) with yours and run
``
make deploy
``

### Direct ConfigMap configuration

You can change default configuration by directly changing the default-config ConfigMap with kubectl like:

 - retrieve the current `default-config` from the cluster

``
kubectl get -n backstage-system configmap default-config > my-config.yaml
``

- modify the file in your editor of choice

- apply the updated configuration to your cluster

``
  kubectl apply -n backstage-system -f my-config.yaml
``

It has to be re-applied to the controller's container after being reconciled by kubernetes processes.

### Recommended Namespace for Operator Installation
It is recommended to deploy the Backstage Operator in a dedicated default namespace `backstage-system`. The cluster administrator can restrict access to the operator resources through RoleBindings or ClusterRoleBindings. On OpenShift, you can choose to deploy the operator in the `openshift-operators` namespace instead. However, you should keep in mind that the Backstage Operator shares the namespace with other operators and therefore any users who can create workloads in that namespace can get their privileges escalated from all operators' service accounts.

### Use Cases

#### Airgapped environment

When creating the Backstage CR, the Operator will try to create a Backstage Pod, deploying:
- Backstage Container from the image, configured in *(deployment.yaml).spec.template.spec.Containers[].image*
- Init Container (applied for RHDH configuration, usually the same as Backstage Container)

Also, if Backstage CR configured with *EnabledLocalDb*,  it will create a PostgreSQL container pod, configured in *(db-deployment.yaml).spec.template.spec.Containers[].image*

By default, the Backstage Operator is configured to use publicly available images.
If you plan to deploy to a [restricted environment](https://docs.openshift.com/container-platform/4.14/operators/admin/olm-restricted-networks.html),
you will need to configure your cluster or network to allow these images to be pulled.
For the list of related images deployed by the Operator, see the `RELATED_IMAGE_*` env vars or `relatedImages` section of the [CSV](../bundle/manifests/backstage-operator.clusterserviceversion.yaml).
See also https://docs.openshift.com/container-platform/4.14/operators/admin/olm-restricted-networks.html


### Installing Operator on Openshift cluster
https://docs.openshift.com/container-platform/4.15/operators/admin/olm-adding-operators-to-cluster.html 