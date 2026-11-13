# rhdh-operator

Kubernetes operator for Red Hat Developer Hub (RHDH), based on the Backstage operator.
Manages `Backstage` CRs and reconciles them into Deployments, ConfigMaps, and related resources.

## Build & Test Commands

- Build: `make build`
- Test (unit and integration tests not requiring real cluster): `make test`
- Integration test (on running cluster and controller): `make integration-test PROFILE=backstage.io USE_EXISTING_CLUSTER=true USE_EXISTING_CONTROLLER=true`
- Lint: `make lint` (golangci-lint + yamllint)
- Lint fix: `make lint-fix`
- Format: `make fmt` (goimports)
- Security scan: `make gosec`
- Generate CRD/deepcopy: `make manifests generate`
- Single-file lint: `golangci-lint run ./path/to/package/...`
- Single-file vet: `go vet ./path/to/package/...`

## Key Conventions

<!-- TODO (maintainers): Add 2-3 conventions an agent couldn't discover by reading the code.
     Examples: naming rules, Kubernetes-specific patterns, things that always break if missed.
     See best_practices.md for patterns already captured from PR reviews. -->

## Architecture

- Design and configuration overview: `docs/design.md`

<!-- TODO (maintainers): Document non-obvious architectural decisions or unexpected code locations.
     Examples: "reconciler logic is split across pkg/ and internal/", "CRD validation happens in the webhook not the controller", invariants that must hold across the reconcile loop. -->

## Pattern References

- CR examples and dependent resources: `examples/`
- Operator coding patterns (from PR reviews): `best_practices.md`
- Design and configuration docs: `docs/`

## PR Conventions

- PR description must link the related issue (`Fixes #<issue>`) and include test and documentation checkboxes.
- Agent-assisted commits should include an `Assisted-by: <model>` footer.
