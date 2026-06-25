---
paths:
  - "internal/controller/**/*.go"
---

# Controller Rules

- Reconcile logic lives in `internal/controller/backstage_controller.go`.
- Use `controllerutil.CreateOrUpdate` for idempotent resource management — avoid plain `Create` which fails on re-reconcile.
- Gate optional features on CRD presence before registering scheme or adding RBAC.
- Do not hard-code `replicas` in any Deployment/StatefulSet template — omit to allow HPA.
- After any controller change, run `make manifests generate fmt vet` before committing.
