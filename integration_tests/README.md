
How to run Integration Tests

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
  
   