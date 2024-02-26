## End-to-end tests

The end-to-end tests use the [Ginkgo framework](https://onsi.github.io/ginkgo/) and allow to test the operator against a real cluster in the following scenarios:
- building and deploying the operator image off of the current code
- using a specific image or a specific downstream build

Deployment of the operator itself can be done by:
- deploying with or without OLM,
- or deploying the downstream bundle in both online and air-gapped scenarios

To run the end-to-end tests, make sure you have an active connection to a cluster in your current [kubeconfig](https://kubernetes.io/docs/concepts/configuration/organize-cluster-access-kubeconfig/) and run:
```shell
# Check your current context
$ kubectl config current-context 
$ make test-e2e
```

### Configuration

The behavior is configurable using the following environment variables:

| Name                                                                                           | Type   | Description                                                                                                                                                                                                                                                                                                                                                                                                                                   | Default value                                     | Example                                                 |
|------------------------------------------------------------------------------------------------|--------|-----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------|---------------------------------------------------|---------------------------------------------------------|
| `BACKSTAGE_OPERATOR_TEST_MODE`                                                                 | string | The test mode:<br>- if not set, it will call `make deploy`<br>- `olm`: it will call `make deploy-olm`<br>- `rhdh-latest` or `rhdh-next`: it will install the operator using the [`install-rhdh-catalog-source.sh`](../../.rhdh/scripts/install-rhdh-catalog-source.sh) script<br>- `rhdh-airgap`: it will install the operator using the [`prepare-restricted-environment.sh`](../../.rhdh/scripts/prepare-restricted-environment.sh) script. |                                                   | `rhdh-latest`                                           |
| `IMG` (or any variables from the Makefile that are used by `make deploy` or `make deploy-olm`) | string | The image to use. Relevant if `BACKSTAGE_OPERATOR_TEST_MODE` is not set or set to `olm`                                                                                                                                                                                                                                                                                                                                                       | `VERSION` defined in [`Makefile`](../../Makefile) | `quay.io/janus-idp/operator:0.0.1-latest`               |
| `BACKSTAGE_OPERATOR_TESTS_BUILD_IMAGES`                                                        | bool   | If set to `true`, it will build the operator image with `make image-build`.<br>Relevant if `BACKSTAGE_OPERATOR_TEST_MODE` is not set or set to `olm`.                                                                                                                                                                                                                                                                                         |                                                   | `false`                                                 |
| `BACKSTAGE_OPERATOR_TESTS_PUSH_IMAGES`                                                         | bool   | If set to `true`, it will push the operator image with `make image-push`.<br>Relevant if `BACKSTAGE_OPERATOR_TEST_MODE` is not set or set to `olm`.                                                                                                                                                                                                                                                                                           |                                                   | `false`                                                 |
| `BACKSTAGE_OPERATOR_TESTS_PLATFORM`                                                            | string | The platform type, to directly load the operator image if supported instead of pushing it.<br>Relevant if `BACKSTAGE_OPERATOR_TEST_MODE` is not set or set to `olm`.br>Supported values: [`kind`](#building-and-testing-local-changes-on-kind), [`k3d`](#building-and-testing-local-changes-on-k3d), [`minikube`](#building-and-testing-local-changes-on-minikube)                                                                            |                                                   | `kind`                                                  |
| `BACKSTAGE_OPERATOR_TESTS_KIND_CLUSTER`                                                        | string | Name of the local KinD cluster to use. Relevant only if `BACKSTAGE_OPERATOR_TESTS_PLATFORM` is `kind`.                                                                                                                                                                                                                                                                                                                                        | `kind`                                            | `kind-local-k8s-cluster`                                |
| `BACKSTAGE_OPERATOR_TESTS_K3D_CLUSTER`                                                         | string | Name of the local k3d cluster to use. Relevant only if `BACKSTAGE_OPERATOR_TESTS_PLATFORM` is `k3d`.                                                                                                                                                                                                                                                                                                                                          | `k3s-default`                                     | `k3d-local-k8s-cluster`                                 |
| `BACKSTAGE_OPERATOR_TESTS_AIRGAP_INDEX_IMAGE`                                                  | string | Index image to use in the airgap scenario.<br>Relevant if `BACKSTAGE_OPERATOR_TEST_MODE` is `rhdh-airgap`.                                                                                                                                                                                                                                                                                                                                    | `quay.io/rhdh/iib:latest-v4.14-x86_64`            | `registry.redhat.io/redhat/redhat-operator-index:v4.14` |
| `BACKSTAGE_OPERATOR_TESTS_AIRGAP_OPERATOR_VERSION`                                             | string | Operator version to use in the airgap scenario.<br>Relevant if `BACKSTAGE_OPERATOR_TEST_MODE` is `rhdh-airgap`.                                                                                                                                                                                                                                                                                                                               | `v1.1.0`                                          | `v1.1.0`                                                |
| `BACKSTAGE_OPERATOR_TESTS_AIRGAP_MIRROR_REGISTRY`                                              | string | Existing mirror registry to use in the airgap scenario.<br>Relevant if `BACKSTAGE_OPERATOR_TEST_MODE` is `rhdh-airgap`<br>.                                                                                                                                                                                                                                                                                                                   |                                                   | `my-registry.example.com`                               |

### Examples

#### Testing the operator available for the VERSION (default)

In this scenario, you want to run the E2E test suite against the operator image corresponding to the `VERSION` declared in the project [`Makefile`](../../Makefile), which should be publicly available at `quay.io/janus-idp/operator:<VERSION>`.

This is the default behavior.

This should work on any Kubernetes or OpenShift cluster:

```shell
$ make test-e2e
```

#### Testing a specific image (e.g. PR image)

In this scenario, you want to run the E2E test suite against an existing operator image.

This should work on any Kubernetes or OpenShift cluster:

```shell
# if the tag is already published and available at the default location: quay.io/janus-idp/operator
$ make test-e2e VERSION=0.2.0-3d1c1e0

# or you can override the full image repo name
$ make test-e2e IMG=my.registry.example.com/operator:0.2.0-3d1c1e0
```

Note that `VERSION` and `IMG` override the respective variables declared in the project [`Makefile`](../../Makefile).

#### Building and testing local changes on supported local clusters

In this scenario, you are iterating locally, and want to run the E2E test suite against your local changes. You are already using a local cluster like [`kind`](https://kind.sigs.k8s.io/), [`k3d`](https://k3d.io/) or [`minikube`](https://minikube.sigs.k8s.io/docs/), which provide the ability to import images into the cluster nodes.

To do so, you can:
1. set `BACKSTAGE_OPERATOR_TESTS_BUILD_IMAGES` to `true`, which will result in building the operator image from the local changes,
2. and set `BACKSTAGE_OPERATOR_TESTS_PLATFORM` to a supported local cluster, which will result in loading the image built directly in that cluster (without having to push to a separate registry).

##### `kind`

```shell
$ kind create cluster
$ make test-e2e \
    BACKSTAGE_OPERATOR_TESTS_BUILD_IMAGES=true \
    BACKSTAGE_OPERATOR_TESTS_PLATFORM=kind
```

##### `k3d`

```shell
$ k3d cluster create
$ make test-e2e \
    BACKSTAGE_OPERATOR_TESTS_BUILD_IMAGES=true \
    BACKSTAGE_OPERATOR_TESTS_PLATFORM=k3d
```

##### `minikube`

```shell
$ minikube start
$ make test-e2e \
    BACKSTAGE_OPERATOR_TESTS_BUILD_IMAGES=true \
    BACKSTAGE_OPERATOR_TESTS_PLATFORM=minikube
```

#### Testing a specific version using OLM

In this scenario, you want to leverage the [Operator Lifecycle Manager (OLM)](https://olm.operatorframework.io/) to deploy the Operator.

This requires OLM to be installed in the cluster.

```shell
$ make test-e2e BACKSTAGE_OPERATOR_TEST_MODE=olm
```

#### Testing a downstream build of Red Hat Developer Hub (RHDH)

In this scenario, you want to run the E2E tests against a downstream build of RHDH.

This works only against OpenShift clusters. So make sure you are logged in to the OpenShift cluster using the `oc` command. See [Logging in to the OpenShift CLI](https://docs.openshift.com/container-platform/4.14/cli_reference/openshift_cli/getting-started-cli.html#cli-logging-in_cli-developer-commands) for more details.

You can check your current context by running `oc config current-context` or `kubectl config current-context`.

If testing a CI build, please follow the instructions in [Installing CI builds of Red Hat Developer Hub](../../.rhdh/docs/installing-ci-builds.adoc) to add your Quay token to the cluster.

```shell
# latest
$ make test-e2e BACKSTAGE_OPERATOR_TEST_MODE=rhdh-latest

# or next
$ make test-e2e BACKSTAGE_OPERATOR_TEST_MODE=rhdh-next
```

#### Airgap testing of Red Hat Developer Hub (RHDH)

In this scenario, you want to run the E2E tests against an OpenShift cluster running in a restricted network. For this, the command below will make sure to prepare it by copying all the necessary images to a mirror registry, then deploy the operator.

Make sure you are logged in to the OpenShift cluster using the `oc` command. See [Logging in to the OpenShift CLI](https://docs.openshift.com/container-platform/4.14/cli_reference/openshift_cli/getting-started-cli.html#cli-logging-in_cli-developer-commands) for more details.

You can check your current context by running `oc config current-context` or `kubectl config current-context`.

Also make sure to read the prerequisites in [Installing Red Hat Developer Hub (RHDH) in restricted environments](../../.rhdh/docs/airgap.adoc).

```shell
# if you want to have a mirror registry to be created for you as part of the airgap environment setup
$ make test-e2e BACKSTAGE_OPERATOR_TEST_MODE=rhdh-airgap

# or if you already have a mirror registry available and reachable from within your cluster
$ make test-e2e \
    BACKSTAGE_OPERATOR_TEST_MODE=rhdh-airgap \
    BACKSTAGE_OPERATOR_TESTS_AIRGAP_MIRROR_REGISTRY=my-registry.example.com
```
