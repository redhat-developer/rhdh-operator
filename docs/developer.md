# DEVELOPER GUIDE --- WIP

## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/)
which provides a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.


## Local development

### Prerequisites

* **kubectl**. See [Instaling kubectl](https://kubernetes.io/docs/tasks/tools/#kubectl).
* **minikube**. See [Instaling minkube](https://kubernetes.io/docs/tasks/tools/#minikube).
* A copy of the Backstage Operator sources:
```sh
git clone https://github.com/janus-idp/operator
```

### Local Tests

To run both unit tests (since 0.0.2) and Kubernetes integration tests ([envtest](https://book.kubebuilder.io/reference/envtest.html)):

```sh
make test
```

### Test on the local cluster

You’ll need a Kubernetes cluster to run against.
You can use [minikube](https://kubernetes.io/docs/tasks/tools/#minikube) or [kind](https://kubernetes.io/docs/tasks/tools/#kind) to get a local cluster for testing, or run against a remote cluster.

**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

- Build and push your image to the location specified by `IMG`:
```sh
make image-build image-push IMG=<your-registry>/backstage-operator:tag
```

- Install the CRDs into the local cluster (minikube is installed and running):
```sh
make install
```

-  You can run your controller standalone (this will run in the foreground, so switch to a new terminal if you want to leave it running)
This way you can see controllers log just in your terminal window which is quite convenient for debugging:
```sh
make run
```

- Or deploy the controller to the cluster with the image specified by `IMG`:
```sh
make deploy IMG=<your-registry>/backstage-operator:tag
```

### Uninstall CRDs
To delete the CRDs from the cluster:
```sh
make uninstall
```
### Undeploy controller
UnDeploy the controller from the cluster:
```sh
make undeploy
```
### Build and Deploy with OLM:
1. To build operator, bundle and catalog images:
```sh
make release-build
```
2. To push operator, bundle and catalog images to the registry:
```sh
make release-push
```
3. To deploy or update catalog source:
```sh
make catalog-update
```
4. To deloy the operator with OLM
```sh
make deploy-olm
```
5. To undeploy the operator with OLM
```sh
make undeploy-olm
```

6. To deploy the operator to Openshift with OLM
```sh
make deploy-openshift [IMAGE_TAG_BASE=<your-registry>/backstage-operator]
```

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:
```sh
make manifests
```
**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)
