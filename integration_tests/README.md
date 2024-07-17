
**How to run Integration Tests**

- For development (controller will reconsile internally)
  - As a part of the whole testing suite just:
  
   `make test`
  - Standalone, just integration tests:
  
   `make integration-test`
   
- For QE (integration/e2e testing). No OLM
  There are 2 environment variables to use with `make` command
  - `USE_EXISTING_CLUSTER=true` tells test suite to use externally running cluster (from the current .kube/config context) instead of envtest.
  - `USE_EXISTING_CONTROLLER=true` tells test suite to use operator controller manager either deployed to the cluster OR (prevails if both) running locally with `make [install] run` command. Works only with `USE_EXISTING_CLUSTER=true`

  So, in most of the cases
  - Make sure you test desirable version of Operator image, that's what
  `make image-build image-push` does. See Makefile what version `<your-mage>` has.
  - Prepare your cluster with:
    - `make install deploy` this will install CR and deploy Controller to `backstage-system`
    - `make integration-test USE_EXISTING_CLUSTER=true USE_EXISTING_CONTROLLER=true`
  
To run GINKGO with command line arguments (see https://onsi.github.io/ginkgo/#running-specs)
use 'ARGS' environment variable.
For example to run specific test(s) you can use something like:

`make integration-test ARGS='--focus "my favorite test"'`

**NOTE:**

Some tests are Openshift specific only and skipped in a local envtest and bare k8s cluster.

`
if !isOpenshiftCluster() {
Skip("Skipped for non-Openshift cluster")
}
`

Some tests are workable only in real (EXISTING) cluster and skipped in envtest.

`
if !*testEnv.UseExistingCluster {
Skip("Skipped for not real cluster")
}
`