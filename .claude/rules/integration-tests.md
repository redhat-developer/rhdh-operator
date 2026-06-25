---
paths:
  - "integration_tests/**/*.go"
  - "tests/**/*.go"
---

# Integration & E2E Test Rules

- Integration tests require a running Kubernetes cluster. Use `make integration-test USE_EXISTING_CLUSTER=true USE_EXISTING_CONTROLLER=true`.
- The `PROFILE` variable selects the test suite: `backstage.io` (faster, default for CI) or `rhdh`.
- Staticcheck (ST1001) is excluded for test files — dot-imports are allowed.
- Use `--focus` to run a single test: `make integration-test ARGS='--focus "test name"' USE_EXISTING_CLUSTER=true USE_EXISTING_CONTROLLER=true`.
