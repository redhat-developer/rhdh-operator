
<b>Pattern 1: Add complete, runnable examples in docs and example manifests by including required dependent resources (for example, Secrets or CRDs) with safe placeholder values so users can apply them out-of-the-box.</b>

Example code before:
```
# examples/orchestrator.yaml
apiVersion: rhdh.redhat.com/v1alpha4
kind: Backstage
metadata:
  name: orchestrator
spec:
  application:
    extraEnvs:
      secrets:
        - name: backend-auth-secret
# Secret referenced above is missing
```

Example code after:
```
# examples/orchestrator.yaml
---
apiVersion: v1
kind: Secret
metadata:
  name: backend-auth-secret
stringData:
  BACKEND_SECRET: "dummy-not-secret"
---
apiVersion: rhdh.redhat.com/v1alpha4
kind: Backstage
metadata:
  name: orchestrator
spec:
  application:
    extraEnvs:
      secrets:
        - name: backend-auth-secret
```

<details><summary>Examples for relevant past discussions:</summary>

- https://github.com/redhat-developer/rhdh-operator/pull/1567#discussion_r2315417933
- https://github.com/redhat-developer/rhdh-operator/pull/1219#discussion_r2163204987
</details>


___

<b>Pattern 2: When introducing optional features controlled by CR fields, implement idempotent create-or-update logic, register required schemes, add RBAC, and gate behavior on CRD presence; also document enabling/disabling and lifecycle management.</b>

Example code before:
```
// creates ServiceMonitor with server-side apply (SSA)
err := c.Patch(ctx, sm, client.Apply, applyOpts)
// no scheme registration or RBAC for ServiceMonitor
```

Example code after:
```
// register monitoring v1 scheme and add RBAC for servicemonitors
controllerutil.CreateOrUpdate(ctx, c, sm, func() error {
  sm.Spec = desiredSpec
  return controllerutil.SetControllerReference(owner, sm, scheme)
})
// check CRD exists if needed; reconcile create/update/delete on spec.monitoring.enabled
```

<details><summary>Examples for relevant past discussions:</summary>

- https://github.com/redhat-developer/rhdh-operator/pull/1374#discussion_r2248149438
- https://github.com/redhat-developer/rhdh-operator/pull/1374#discussion_r2253372015
- https://github.com/redhat-developer/rhdh-operator/pull/1499#discussion_r2284826017
</details>


___

<b>Pattern 3: Preserve autoscaling compatibility by omitting or commenting out hard-coded replicas in Deployment/StatefulSet templates and add explicit comments explaining the omission.</b>

Example code before:
```
spec:
  replicas: 1
  template:
    spec: {}
```

Example code after:
```
spec:
  # replicas: 1  # Intentionally omitted to allow HPA or custom scaling control.
  template:
    spec: {}
```

<details><summary>Examples for relevant past discussions:</summary>

- https://github.com/redhat-developer/rhdh-operator/pull/1284#discussion_r2156758328
- https://github.com/redhat-developer/rhdh-operator/pull/1284#discussion_r2157238898
</details>


___

<b>Pattern 4: Keep documentation synchronized with implementation changes, specifying versions, defaults, namespaces, and merge semantics to avoid user confusion when behavior evolves.</b>

Example code before:
```
# docs/configuration.md
From version 0.7.0, dynamic plugins are overridden by the CR.
```

Example code after:
```
# docs/configuration.md
Before 0.8.0 the Operator replaced defaults; since 0.8.0 it merges defaults with the user ConfigMap (non-deep merge).
Resources are created in the same namespace as the Backstage CR unless stated otherwise.
```

<details><summary>Examples for relevant past discussions:</summary>

- https://github.com/redhat-developer/rhdh-operator/pull/1486#discussion_r2288776329
- https://github.com/redhat-developer/rhdh-operator/pull/1551#discussion_r2301210631
- https://github.com/redhat-developer/rhdh-operator/pull/1551#discussion_r2301214220
- https://github.com/redhat-developer/rhdh-operator/pull/1323#discussion_r2179583371
</details>


___

<b>Pattern 5: Harden shell scripts by enabling strict modes, quoting variables and arrays, validating required env vars, avoiding brittle traps, and removing unused variables to satisfy ShellCheck and prevent runtime errors.</b>

Example code before:
```
#!/bin/bash
for db in ${!allDB[@]}; do
  echo Copying database: $db
done
trap "rm -f $tmpFile" EXIT
```

Example code after:
```
#!/bin/bash
set -euo pipefail
: "${TO_PSW:?TO_PSW environment variable not set}"
for db in "${allDB[@]}"; do
  echo "Copying database: ${db}"
done
trap 'rm -f "$tmpFile" || true' EXIT
```

<details><summary>Examples for relevant past discussions:</summary>

- https://github.com/redhat-developer/rhdh-operator/pull/1305#discussion_r2175117965
- https://github.com/redhat-developer/rhdh-operator/pull/1305#discussion_r2175123942
- https://github.com/redhat-developer/rhdh-operator/pull/1305#discussion_r2175338254
- https://github.com/redhat-developer/rhdh-operator/pull/1305#discussion_r2188582064
- https://github.com/redhat-developer/rhdh-operator/pull/1305#discussion_r2188591953
</details>


___
