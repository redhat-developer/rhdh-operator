# Developer Guide 

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

## Local development

### Prerequisites

* **kubectl**. See [Instaling kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl).
* Available local or remote Kubernetes cluster with cluster admin privileges. For instance **minikube**. See [Instaling minkube](https://kubernetes.io/docs/tasks/tools/#minikube).
* A copy of the Backstage Operator sources:
```sh
git clone https://github.com/redhat-developer/rhdh-operator
```

### Local Tests

To run:
* all the unit tests 
* part of [Integration Tests](../integration_tests/README.md) which does not require a real cluster.

```sh
make test
```

It only takes a few seconds to run, but covers quite a lot of functionality. For early regression detection, it is recommended to run it as often as possible during development.

### Test on the cluster

For testing, you will need a Kubernetes cluster, either remote (with sufficient admin rights) or local, such as [minikube](https://kubernetes.io/docs/tasks/tools/#minikube) or [kind](https://kubernetes.io/docs/tasks/tools/#kind)

- Build and push your image to the location specified by `IMG`:
```sh
make image-build image-push IMG=<your-registry>/backstage-operator:tag
```

**NOTE:** This image ought to be published in the personal registry you specified.
And it is required to have access to pull the image from the working environment.
Make sure you have the proper permission to the registry if the above commands don’t work.

- Install the Custom Resource Definitions into the local cluster (minikube is installed and running):
```sh
make install
```
**IMPORTANT:** If you are editing the CRDs, make sure you reinstall it before deploying.

- To delete the CRDs from the cluster:
```sh
make uninstall
```

### Run the controller standalone

You can run your controller standalone (this will run in the foreground, so switch to a new terminal if you want to leave it running)
This way you can see controllers log just in your terminal window which is quite convenient for debugging.

```sh
make [PROFILE=<configuration-profile>] [install] run
```

You can use it for manual and automated ([such as](../integration_tests/README.md) `USE_EXISTING_CLUSTER=true make integration-test`) tests efficiently, but, note, RBAC is not working with this kind of deployment.

### Deploy operator to the real cluster

#### Configuration Profiles

Since v0.3.0 Operator has a facility to support different predefined runtime configurations, we call it Configuration Profile.
You can see them as a subdirectories of /config/profile: 

* **default** (default as for v0.3.0) - OOTB supporting Red Hat Developer Hub runtime
* **backstage.io** - bare backstage image
* **external** - empty profile you can feed your configuration from outside (instructions TBD)

#### Deploy

To deploy the Operator directly to current cluster use:
```sh
make deploy [PROFILE=<configuration-profile>] [IMG=<your-registry>/backstage-operator[:tag]]
```

> **NOTE**: If you encounter RBAC errors, you may need to grant yourself cluster-admin
privileges or be logged in as admin.

To undeploy the controller from the cluster:
```sh
make undeploy
```

**UnDeploy the controller from the cluster:**

```sh
make undeploy
```

### Deploy with Operator Lifecycle Manager (valid for v0.3.0+):

#### OLM

Make sure your cluster supports **OLM**. For instance [Openshift](https://www.redhat.com/en/technologies/cloud-computing/openshift) supports it out of the box.
If needed install it using: 

```sh
make install-olm
```

#### Build and push images

There are a bunch of commands to build and push to the registry necessary images.
For development purpose, most probably, you will need to specify the image you build and push with IMAGE_TAG_BASE env variable: 

* `[IMAGE_TAG_BASE=<your-registry>/backstage-operator] make image-build` builds operator manager image (**backstage-operator**)
* `[IMAGE_TAG_BASE=<your-registry>/backstage-operator] make image-push` pushes operator manager image to **your-registry**
* `[IMAGE_TAG_BASE=<your-registry>/backstage-operator] make bundle-build` builds operator manager image (**backstage-operator-bundle**)
* `[IMAGE_TAG_BASE=<your-registry>/backstage-operator] make bundle-push` pushes bundle image to **your-registry**
* `[IMAGE_TAG_BASE=<your-registry>/backstage-operator] make catalog-build` builds catalog image (**backstage-operator-catalog**)
* `[IMAGE_TAG_BASE=<your-registry>/backstage-operator] make catalog-push` pushes catalog image to **your-registry**

You can do it all together using:
```sh
[IMAGE_TAG_BASE=<your-registry>/backstage-operator] make release-build release-push
```

#### Deploy or update the Catalog Source

```sh
[OLM_NAMESPACE=<olm-namespace>] [IMAGE_TAG_BASE=<your-registry>/backstage-operator] make catalog-update
```
You can point the namespace where OLM installed. By default, in a vanilla Kubernetes, OLM os deployed on 'olm' namespace. In Openshift you have to explicitly point it to **openshift-marketplace** namespace.

#### Deploy the Operator with OLM 
Default namespace to deploy the Operator is called **backstage-system** , this name fits one defined in [kustomization.yaml](../config/default/kustomization.yaml). So, if you consider changing it you have to change it in this file and define **OPERATOR_NAMESPACE** environment variable.
Following command creates OperatorGroup and Subscription on Operator namespace
```sh
[OPERATOR_NAMESPACE=<operator-namespace>] make deploy-olm
```
To undeploy the Operator
```sh
make undeploy-olm
```

#### Convenient commands to build and deploy operator with OLM 

**NOTE:** OLM has to be installed as a prerequisite

* To build and deploy the operator to vanilla Kubernetes with OLM
```sh
[IMAGE_TAG_BASE=<your-registry>/backstage-operator] make deploy-k8s-olm
```

* To build and deploy the operator to Openshift with OLM
```sh
[IMAGE_TAG_BASE=<your-registry>/backstage-operator] make deploy-openshift 
```


## Project Distribution

Following are the steps to build the installer and distribute this project to users.

1. Build the installer for the image built and published in the registry:

```sh
make build-installer IMG=<some-registry>/backstage-operator:tag [PROFILE=rhdh]
```

NOTE: The makefile target mentioned above generates an 'install.yaml'
file in the `dist/${PROFILE}` directory. This file contains all the resources built
with Kustomize, which are necessary to install this project without
its dependencies.

2. Using the installer

Users can just run `kubectl apply -f <URL for YAML BUNDLE>` to install the project, i.e.:

```sh
kubectl apply -f https://raw.githubusercontent.com/<org>/rhdh-operator/<tag or branch>/dist/install.yaml
```

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
