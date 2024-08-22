# Developer Guide 

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project


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
make [install] run
```

You can use it for manual and automated ([such as](../integration_tests/README.md) `USE_EXISTING_CLUSTER=true make integration-test`) tests efficiently, but, note, RBAC is not working with this kind of deployment.

### Deploy operator to the real cluster

For development, most probably, you will need to specify the image you build and push:
```sh
make deploy [IMG=<your-registry>/backstage-operator[:tag]]
```

To undeploy the controller from the cluster:
```sh
make undeploy
```

- To generate deployment manifest, use:
```sh
make deployment-manifest [IMG=<your-registry>/backstage-operator:tag]
```
it will create the file rhdh-operator-${VERSION}.yaml on the project root and you will be able to share it to make it possible to deploy operator with:
```sh
kubectl apply -f <path-or-url-to-deployment-script>
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


**NOTE:** Run `make help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

