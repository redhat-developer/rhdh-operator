---
paths:
  - "api/**/*.go"
---

# API Types Rules

- New API versions go in `api/v1alphaN/` following the existing version directories.
- All exported types and fields must have godoc comments (Go convention: comment starts with the symbol name).
- After modifying any type in `api/`, run `make manifests generate` — the generated CRD YAML and deepcopy code must be committed with the source change.
- Validation markers (`+kubebuilder:validation:*`) go on struct fields, not separately.
- Check `api/current-types.go` to understand which API version is current/promoted.
