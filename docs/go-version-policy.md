# Go Versioning Policy for the RHDH Operator

## TL;DR

| Control | Main branch | Release branches |
|---|---|---|
| `go` directive in `go.mod` | Bump to match the latest Go version available in go-toolset | Frozen at branch-cut value; do not bump |
| `toolchain` directive in `go.mod` | Update to the latest patch release of the declared `go` version | Patch-level updates via Renovate while the declared `go` version is supported |
| go-toolset builder image | Track the latest available image | Track the latest available image |
| `constraints.go` in Renovate config | Must match the `go` directive value | Must match the `go` directive value on that branch |
| Dependency updates requiring a `go` bump | Allowed if go-toolset has the required version | Skip the update; escalate to Security if it is a CVE fix |

## Purpose and Scope

This policy governs how the Install team manages Go versions in the [rhdh-operator](https://github.com/redhat-developer/rhdh-operator) repository. It applies to all maintainers who review or approve changes to the operator's `go.mod` file, its Dockerfile base images, or the Renovate configuration that automates updates to both.

The operator's build process involves two independent Go version controls. The [`go` directive](https://go.dev/doc/modules/gomod-ref) in `go.mod` declares the minimum Go language version required by the module's source code. The [go-toolset](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690) container image provides the Go compiler and standard library that are linked into the shipped binary. These two controls are deliberately decoupled and follow different update rules. Understanding the distinction is essential to applying this policy correctly; the Go project's [toolchain management documentation](https://go.dev/doc/toolchain) explains the underlying mechanism in detail.

## Language Version Policy on the Main Branch

The `go` directive on the main branch is eventually bumped to match the latest Go version available in the Red Hat [go-toolset](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690) image, which in practice can lag behind the latest upstream Go release. This lag is not a policy choice but a consequence of the go-toolset image availability: the Red Hat go-toolset typically may lag behind official upstream Go releases by several months while the image is rebuilt and certified against the current RHEL base. See https://access.redhat.com/articles/7116095.

A new Go version is considered available for the operator when a go-toolset image containing that version appears in the [Red Hat Ecosystem Catalog](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690). Maintainers can monitor availability by checking the catalog for new tags or by observing when Renovate opens a Docker minor update pull request for the go-toolset image in the operator repository. Updates to the `go` directive are driven by this availability, not by upstream Go release dates.

When the `go` directive is bumped, the `toolchain` directive should be updated to track the latest patch release of the declared version. Both changes are deliberate and tracked via a Jira issue each release cycle (for example, [RHIDP-12020](https://redhat.atlassian.net/browse/RHIDP-12020) tracked the update to Go 1.26). At the same time, the [`constraints.go` setting](https://docs.renovatebot.com/golang/#go-binary-version) in the Renovate configuration at [`.github/renovate.json`](../.github/renovate.json) must be updated to match the new `go` directive value. This constraint controls which Go dependency versions Renovate is permitted to propose, and a mismatch will cause Renovate to offer updates that may be incompatible with the declared language version.

It is also acceptable to update the `go` and `toolchain` directives on the main branch before the scheduled release cycle bump if a dependency update requires a newer Go version and that version is available in go-toolset.

## Language Version Policy on Release Branches

The `go` directive on release branches is frozen at the value present when the branch was cut. Maintainers must not bump the `go` directive on release branches, even if the declared Go version becomes unsupported by the upstream Go project. The `toolchain` directive is also set at branch-cut time, but patch-level updates (for example, `go1.25.9` to `go1.25.10`) are permitted and automated by Renovate for as long as the declared `go` version remains supported.

This is safe because the Go version declared in `go.mod` does not determine the standard library linked into the shipped binary. The binary's standard library comes from the go-toolset compiler used at build time. As long as the go-toolset image is kept current (see the next section), the shipped binary continues to receive Go standard library and compiler security patches regardless of what `go.mod` declares.

The [GODEBUG mechanism](https://go.dev/doc/godebug) ensures that a newer compiler building a module with an older `go` directive will respect the behavior semantics of the declared version. When a Go release introduces a backward-incompatible change, the new behavior is gated behind a GODEBUG setting whose default is derived from the `go` directive in `go.mod`. A binary compiled with Go 1.26 against a module declaring `go 1.24` will therefore use the Go 1.24 behavior for any GODEBUG-controlled changes, preserving the expected runtime semantics without requiring a directive bump.

## Build Image Policy

The go-toolset image must track the latest available version on all branches, including release branches. This is the primary mechanism by which the operator's shipped binary receives security patches in the Go standard library and compiler. Holding back the go-toolset image to match the `go.mod` version would deprive release branches of security fixes and is explicitly not part of this policy.

UBI images are rebuilt and distributed on approximately a six-week cadence, or sooner when triggered by a Critical or Important CVE. The go-toolset image inherits this cadence. When a new RHEL minor version is released, the go-toolset image is rebuilt on the updated UBI, and the image tag reflects the new RHEL version. These updates may also include a newer Go compiler version.

The operator's Dockerfile references the go-toolset image from the unauthenticated registry (currently `registry.access.redhat.com/ubi9/go-toolset`; the UBI base version will change when the project transitions to a newer UBI). The `#@follow_tag` annotation in the Dockerfile indicates the corresponding authenticated registry path and is used by automated tooling to track the latest available tag. The image reference is pinned by digest for reproducibility; the digest is updated when a new go-toolset image becomes available.

The runtime stage of the Dockerfile uses a UBI minimal image, a stripped-down UBI variant that includes only `microdnf` and a minimal set of packages. The compiled operator binary is copied from the builder stage into this minimal image, keeping the final shipped image small and reducing the attack surface.

A version gap between the `go.mod` directive and the go-toolset compiler is expected and intentional. On release branches, it is a direct consequence of freezing `go.mod` while continuing to update the build image, and it is the mechanism that allows release branches to remain secure without destabilizing the module's declared compatibility. This gap can also exist temporarily on the main branch until the scheduled `go` directive bump task is implemented (for example, go-toolset may ship Go 1.26 while main still declares `go 1.25` until the bump is completed).

## Dependency Updates on Release Branches

When a dependency update on a release branch requires bumping the `go` directive in `go.mod` to a newer Go version, the update should be skipped. This commonly occurs with pseudo-versioned dependencies tied to newer upstream Git commits that have adopted a higher Go language version. Skipping the update preserves the stability of the release branch's declared compatibility surface.

The exception to this rule is a critical security fix in a dependency. If a security vulnerability in a dependency can only be resolved by pulling in a version that requires a newer `go` directive, the situation must be evaluated on a case-by-case basis with the RHDH Security team. In such cases, bumping the `go` and `toolchain` directives on the release branch is acceptable if the Security team determines the risk of the vulnerability outweighs the risk of changing the language version on a release branch.

When evaluating whether a dependency security fix truly requires a `go` directive bump, maintainers should check whether the fix is available in an older version of the dependency that remains compatible with the current `go` directive. The bump should be treated as a last resort, not a default response to a dependency update conflict.

## Renovate Behavior and Review Guidance

The operator repository uses [Renovate](https://docs.renovatebot.com/) to automate dependency and base image updates. The Renovate configuration at `.github/renovate.json` defines separate rules for Go dependencies and Docker base images, and reviewers should understand how each category behaves.

Docker base image updates to the go-toolset image are classified by Renovate as minor updates when the RHEL version component of the tag changes (for example, `9.7` to `9.8`). However, a Docker minor update can carry a major Go version change inside the image. The go-toolset `9.7` image shipped Go 1.25, while go-toolset `9.8` ships Go 1.26. The pull request diff will only show the image tag and digest change; it will not indicate the Go version change. Reviewers must check the Go version inside the image before approving, using the `skopeo inspect` command described in the Verification section below.

Docker minor updates to go-toolset are not automerged by the Renovate configuration. They produce a pull request that requires manual review and approval. This is intentional and must not be changed, because these updates may introduce a new Go compiler version that should be verified against the nightly test suite before merging. Docker patch updates (digest-only changes within the same tag), by contrast, are automerged. These patch updates can still carry Go compiler patch bumps, so maintainers should monitor the nightly test results after they merge.

Go dependency updates are governed by the [`constraints.go`](https://docs.renovatebot.com/golang/#go-binary-version) setting in the Renovate configuration. This setting specifies the Go binary version that Renovate uses when running `go mod tidy` and other Go commands to evaluate dependency updates. The value must match the `go` directive in `go.mod` on each branch. When the `go` directive is bumped on the main branch, the `constraints.go` value must be updated in the same change or immediately after. A stale `constraints.go` value will cause Renovate's `go mod tidy` runs to use the wrong Go version, producing incorrect dependency resolutions or failures.

The `go` and `toolchain` directive bumps in `go.mod` are tracked and executed separately from go-toolset image updates. They are deliberate changes managed through the Jira release cycle process, not automated by Renovate. Renovate does automate patch-level updates to the `toolchain` directive (for example, `go1.25.9` to `go1.25.10`) and opens pull requests for these changes. These pull requests require manual review and are not automerged, which gives maintainers an opportunity to verify the patch before it reaches a release branch.

## Compatibility Guarantee

This policy relies on Go's strong backward compatibility guarantees. The [Go 1 compatibility promise](https://go.dev/doc/go1compat) states that `it is intended that programs written to the Go 1 specification will continue to compile and run correctly, unchanged, over the lifetime of that specification.`.

The [GODEBUG mechanism](https://go.dev/doc/godebug), described in the Release Branches section above, provides an additional safety layer. The nightly test suite provides further coverage against compatibility regressions. If a go-toolset update introduces a behavioral change that affects the operator despite the compatibility promise and GODEBUG protection, the nightly tests are expected to surface the issue before it reaches a release.

This policy was first established during the discussion captured in [RHIDP-12347](https://redhat.atlassian.net/browse/RHIDP-12347), when go-toolset shipped Go 1.24 but the release-1.6 branch declared `go 1.22` in its `go.mod`. The conclusion at that time, which this document formalizes, was that building with a newer go-toolset is safe, that the "one version behind" policy applies to the `go` directive and not to the compiler, and that security fixes in the standard library are received through the go-toolset compiler regardless of the `go.mod` version.

## Verification

To check the Go compiler version inside a go-toolset image tag before approving a Renovate pull request, run:

```
skopeo inspect docker://registry.access.redhat.com/ubi9/go-toolset:<tag> | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['Labels']['version'])"
```

Replace `<tag>` with the tag from the pull request (for example, `9.8-1777889793`).

To confirm the Go version linked into a shipped operator binary, inspect the software bill of materials:

```
cosign download sbom --platform=linux/amd64 quay.io/rhdh/rhdh-rhel9-operator:<tag> | \
  jq -r '.packages[] | select(.name == "stdlib") | .versionInfo'
```

Replace `<tag>` with the operator image tag (for example, `1.8` or `1.8-91`).

To check available go-toolset versions and determine when a new Go version has become available for the operator, browse the [Go Toolset](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690) page in the Red Hat Ecosystem Catalog. The catalog lists all available tags and their associated Go versions. Alternatively, query the registry directly using the registry path from the operator's Dockerfile:

```
skopeo inspect docker://registry.access.redhat.com/ubi9/go-toolset:latest | \
  python3 -c "import sys,json; print(json.load(sys.stdin)['Labels']['version'])"
```

## References

- [Go 1 and the Future of Go Programs](https://go.dev/doc/go1compat) — the Go 1 compatibility promise
- [Go, Backwards Compatibility, and GODEBUG](https://go.dev/doc/godebug) — how GODEBUG settings preserve backward compatibility
- [Go Toolchains](https://go.dev/doc/toolchain) — the `go` and `toolchain` directives in `go.mod`
- [Go Release Policy](https://go.dev/doc/devel/release) — Go's release cadence and supported version policy
- [go.mod File Reference](https://go.dev/doc/modules/gomod-ref) — specification of all `go.mod` directives
- [Red Hat UBI: Images, Repositories, Packages, and Source Code](https://access.redhat.com/articles/4238681) — comprehensive UBI reference
- [Universal Base Images FAQ](https://developers.redhat.com/articles/ubi-faq) — licensing, redistribution, and certification requirements
- [Go Toolset for UBI 9 — Red Hat Ecosystem Catalog](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690) — available go-toolset image tags and metadata
- [UBI 9 Minimal — Red Hat Ecosystem Catalog](https://catalog.redhat.com/en/software/containers/ubi9/ubi-minimal/61832888c0d15aff4912fe0d) — the runtime base image
- [RHIDP-14096](https://redhat.atlassian.net/browse/RHIDP-14096) — the Jira issue that prompted this policy document
