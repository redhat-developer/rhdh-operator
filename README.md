# backstage-operator
Operator for deploying Backstage for Janus-IDP.

## Description
Implementing https://janus-idp.io/docs/deployment/k8s/ procedure
At first stage CR update does not affect Backstage Objects, just installation (same as Helm)
TODO: Do we need to continuosly sync the states? Which way if so: from CR to Objects or back or (somehow) back and forth?

Make sure namespace is created.

Local Database (PostgreSQL) is created by default, to disable
spec: 
  skipLocalDb: true
This way third party DB can theorethically be configured. TODO: It just requires some changes in Backstage appConfig (I think), 
because it only expects either in-container SQLite or MySQL.
TODO: should we consider using in-container SQLite for K8s deployment as well (single container deployment)?

TODO: POSTGRES_HOST = <name-of the service> , POSTGRES_PORT = <port>[5432] can be delivered to the Backstage 
Deployment out of Postgres Secret? Indeed, it is not really a secret.

## Getting Started
You’ll need a Kubernetes cluster to run against. You can use [KIND](https://sigs.k8s.io/kind) to get a local cluster for testing, or run against a remote cluster.
**Note:** Your controller will automatically use the current context in your kubeconfig file (i.e. whatever cluster `kubectl cluster-info` shows).

### Running on the cluster
1. Install Instances of Custom Resources:
```sh
kubectl apply -f config/samples/
```
2. Build and push your image to the location specified by `IMG`:
```sh
make docker-build docker-push IMG=<some-registry>/backstage-operator:tag
```
3. Deploy the controller to the cluster with the image specified by `IMG`:
```sh
make deploy IMG=<some-registry>/backstage-operator:tag
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
## Build and Deploy with OLM
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
4. To undeloy the operator with OLM
```sh
make undeploy-olm
```
## Contributing
// TODO(user): Add detailed information on how you would like others to contribute to this project

### How it works
This project aims to follow the Kubernetes [Operator pattern](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/).

It uses [Controllers](https://kubernetes.io/docs/concepts/architecture/controller/) 
which provides a reconcile function responsible for synchronizing resources until the desired state is reached on the cluster.

### Test It Out
1. Install the CRDs into the cluster:
```sh
make install
```
2. Run your controller (this will run in the foreground, so switch to a new terminal if you want to leave it running):
```sh
make run
```
**NOTE:** You can also run this in one step by running: `make install run`

### Modifying the API definitions
If you are editing the API definitions, generate the manifests such as CRs or CRDs using:
```sh
make manifests
```
**NOTE:** Run `make --help` for more information on all potential `make` targets

More information can be found via the [Kubebuilder Documentation](https://book.kubebuilder.io/introduction.html)

## License

Copyright 2023 Red Hat Inc..

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

