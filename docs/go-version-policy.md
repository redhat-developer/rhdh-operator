# Go Versioning Policy for the RHDH Operator

## Purpose and Scope

This policy governs how the Install team manages Go versions in the [rhdh-operator](https://github.com/redhat-developer/rhdh-operator) repository. It applies to all maintainers who review or approve changes to the operator's `go.mod` file, its Dockerfile base images, or the Renovate configuration that automates updates to both.

The operator's build process involves two independent Go version controls. The [`go` directive](https://go.dev/doc/modules/gomod-ref) in `go.mod` declares the minimum Go language version required by the module's source code. The [go-toolset](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690) container image provides the Go compiler and standard library that are linked into the shipped binary. These two controls are deliberately decoupled and follow different update rules. Understanding the distinction is essential to applying this policy correctly; the Go project's [toolchain management documentation](https://go.dev/doc/toolchain) explains the underlying mechanism in detail.

## Definitions

The **[`go` directive](https://go.dev/doc/modules/gomod-ref)** in `go.mod` sets the minimum Go language version for the module. Since Go 1.21, this directive is a mandatory requirement enforced by the toolchain: a Go compiler older than the declared version will refuse to build the module. The directive also controls the default [GODEBUG](https://go.dev/doc/godebug) settings applied to the compiled binary, which determines how the program behaves with respect to backward-incompatible changes introduced in later Go releases.

The **[`toolchain` directive](https://go.dev/doc/toolchain)** in `go.mod` declares a suggested Go toolchain version for building the module. When present, it indicates the specific patch release the maintainers intend for use. When absent, Go treats the module as if it had an implicit `toolchain goV` line matching the `go` directive. The `toolchain` directive only takes effect when the module is the main module and the default toolchain's version is older than the suggested one.

The **[go-toolset](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690)** image is a Red Hat-provided container image that bundles the Go compiler, linker, and standard library on top of a [Universal Base Image (UBI)](https://access.redhat.com/articles/4238681) foundation. It is the image referenced in the operator's Dockerfile `FROM` line for the builder stage. The go-toolset image version (for example, `9.8`) corresponds to the RHEL minor release it is built from, not to the Go version it contains. A go-toolset `9.8` image may ship Go 1.26, while a go-toolset `9.6` image shipped Go 1.24. The Go version inside a given go-toolset tag can be determined using the verification procedures described later in this document.

**[Universal Base Image (UBI)](https://access.redhat.com/articles/4238681)** is Red Hat's freely redistributable, OCI-compliant container base image built from Red Hat Enterprise Linux. UBI images are available without a subscription from the unauthenticated registry at `registry.access.redhat.com` and from the authenticated registry at `registry.redhat.io`. All container images shipped as part of a Red Hat product must be built on UBI to meet [Red Hat's container certification requirements](https://developers.redhat.com/articles/ubi-faq). The operator uses two UBI variants: `ubi9/go-toolset` as the builder stage and [`ubi9-minimal`](https://catalog.redhat.com/en/software/containers/ubi9/ubi-minimal/61832888c0d15aff4912fe0d) as the runtime stage. The UBI content availability and life cycle are governed by the [corresponding RHEL release](https://access.redhat.com/support/policy/updates/ubi).

## Language Version Policy on the Main Branch

The `go` directive on the main branch tracks the latest Go version available in the Red Hat [go-toolset](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690) image, which in practice tends to be one major version behind the latest upstream Go release. This lag is not a policy choice but a consequence of the go-toolset image availability: the Red Hat go-toolset typically lags behind official upstream Go releases by several months while the image is rebuilt and certified against the current RHEL base. Go's [release policy](https://go.dev/doc/devel/release) supports the two most recent major versions, so even with this lag the module remains within the supported window.

A new Go version is considered available for the operator when a go-toolset image containing that version appears in the [Red Hat Ecosystem Catalog](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690). Maintainers can monitor availability by checking the catalog for new tags or by observing when Renovate opens a Docker minor update pull request for the go-toolset image in the operator repository. Updates to the `go` directive are driven by this availability, not by upstream Go release dates.

When the `go` directive is bumped, the `toolchain` directive should be updated to track the latest patch release of the declared version. Both changes are deliberate and tracked via a Jira issue each release cycle (for example, [RHIDP-12020](https://redhat.atlassian.net/browse/RHIDP-12020) tracks the update to Go 1.26). At the same time, the `constraints.go` setting in the Renovate configuration at `.github/renovate.json` must be updated to match the new `go` directive value. This constraint controls which Go dependency versions Renovate is permitted to propose, and a mismatch will cause Renovate to offer updates that are incompatible with the declared language version.

It is also acceptable to update the `go` and `toolchain` directives on the main branch before the scheduled release cycle bump if a dependency update requires a newer Go version and that version is available in go-toolset.

## Language Version Policy on Release Branches

The `go` and `toolchain` directives on release branches are frozen at the values present when the branch was cut. Maintainers must not bump these directives on release branches, even if the declared Go version becomes unsupported by the upstream Go project.

This is safe because the Go version declared in `go.mod` does not determine the standard library linked into the shipped binary. The binary's standard library comes from the go-toolset compiler used at build time. As long as the go-toolset image is kept current (see the next section), the shipped binary continues to receive Go standard library and compiler security patches regardless of what `go.mod` declares.

The [GODEBUG mechanism](https://go.dev/doc/godebug) ensures that a newer compiler building a module with an older `go` directive will respect the behavior semantics of the declared version. When a Go release introduces a backward-incompatible change, the new behavior is gated behind a GODEBUG setting whose default is derived from the `go` directive in `go.mod`. A binary compiled with Go 1.26 against a module declaring `go 1.24` will therefore use the Go 1.24 behavior for any GODEBUG-controlled changes, preserving the expected runtime semantics without requiring a directive bump.

## Build Image Policy

The go-toolset image must track the latest available version on all branches, including release branches. This is the primary mechanism by which the operator's shipped binary receives security patches in the Go standard library and compiler. Holding back the go-toolset image to match the `go.mod` version would deprive release branches of security fixes and is explicitly not part of this policy.

The go-toolset image is built on a [Red Hat Universal Base Image (UBI)](https://access.redhat.com/articles/4238681) foundation. UBI images are rebuilt and distributed on approximately a six-week cadence, or sooner when triggered by a Critical or Important CVE. The go-toolset image inherits this cadence. When a new RHEL minor version is released (for example, RHEL 9.8), the go-toolset image is rebuilt on the updated UBI, and the image tag reflects the new RHEL version. These updates may also include a newer Go compiler version.

The operator's Dockerfile references the go-toolset image from the unauthenticated registry at `registry.access.redhat.com/ubi9/go-toolset`. The `#@follow_tag` annotation in the Dockerfile indicates the corresponding authenticated registry path at `registry.redhat.io/rhel9/go-toolset` and is used by automated tooling to track the latest available tag. The image reference is pinned by digest for reproducibility; the digest is updated when a new go-toolset image becomes available.

The runtime stage of the Dockerfile uses [`ubi9-minimal`](https://catalog.redhat.com/en/software/containers/ubi9/ubi-minimal/61832888c0d15aff4912fe0d), a stripped-down UBI variant that includes only `microdnf` and a minimal set of packages. The compiled operator binary is copied from the builder stage into this minimal image, keeping the final shipped image small and reducing the attack surface.

The version gap between the `go.mod` directive and the go-toolset compiler on release branches is expected and intentional. It is a direct consequence of freezing `go.mod` while continuing to update the build image, and it is the mechanism that allows release branches to remain secure without destabilizing the module's declared compatibility.

## Dependency Updates on Release Branches

When a dependency update on a release branch requires bumping the `go` directive in `go.mod` to a newer Go version, the update should be skipped. This commonly occurs with pseudo-versioned dependencies tied to newer upstream Git commits that have adopted a higher Go language version. Skipping the update preserves the stability of the release branch's declared compatibility surface.

The exception to this rule is a critical security fix in a dependency. If a security vulnerability in a dependency can only be resolved by pulling in a version that requires a newer `go` directive, the situation must be evaluated on a case-by-case basis with the RHDH Security team. In such cases, bumping the `go` and `toolchain` directives on the release branch is acceptable if the Security team determines the risk of the vulnerability outweighs the risk of changing the language version on a release branch.

When evaluating whether a dependency security fix truly requires a `go` directive bump, maintainers should check whether the fix is available in an older version of the dependency that remains compatible with the current `go` directive. The bump should be treated as a last resort, not a default response to a dependency update conflict.

## Renovate Behavior and Review Guidance

The operator repository uses [Renovate](https://docs.renovatebot.com/) to automate dependency and base image updates. The Renovate configuration at `.github/renovate.json` defines separate rules for Go dependencies and Docker base images, and reviewers should understand how each category behaves.

Docker base image updates to the go-toolset image are classified by Renovate as minor updates when the RHEL version component of the tag changes (for example, `9.7` to `9.8`). However, a Docker minor update can carry a major Go version change inside the image. The go-toolset `9.7` image shipped Go 1.25, while go-toolset `9.8` ships Go 1.26. The pull request diff will only show the image tag and digest change; it will not indicate the Go version change. Reviewers must check the Go version inside the image before approving, using the `skopeo inspect` command described in the Verification section below.

Docker minor updates to go-toolset are not automerged by the Renovate configuration. They produce a pull request that requires manual review and approval. This is intentional and must not be changed, because these updates may introduce a new Go compiler version that should be verified against the nightly test suite before merging.

Go dependency updates are governed by the `constraints.go` setting in the Renovate configuration. This setting restricts the Go versions that Renovate will consider when evaluating whether a dependency update is compatible. The value must match the `go` directive in `go.mod` on each branch. When the `go` directive is bumped on the main branch, the `constraints.go` value must be updated in the same change or immediately after. A stale `constraints.go` value will either block valid updates or allow incompatible ones.

The `go` and `toolchain` directive bumps in `go.mod` are tracked and executed separately from go-toolset image updates. They are deliberate changes managed through the Jira release cycle process, not automated by Renovate. Renovate does automate patch-level updates to the `toolchain` directive (for example, `go1.25.9` to `go1.25.10`) and opens pull requests for these changes. These pull requests require manual review and are not automerged, which gives maintainers an opportunity to verify the patch before it reaches a release branch.

## Compatibility Guarantee

This policy relies on Go's strong backward compatibility guarantees. The [Go 1 compatibility promise](https://go.dev/doc/go1compat) states that programs written to the Go 1 specification will continue to compile and run correctly across future Go 1.x releases. Each release note since Go 1.0 has reiterated this promise, typically with the statement: "As always, the release maintains the Go 1 promise of compatibility. We expect almost all Go programs to continue to compile and run as before."

The [GODEBUG mechanism](https://go.dev/doc/godebug) provides an additional safety layer. When a Go release introduces a change that could affect existing programs, the change is gated behind a GODEBUG setting. The default value of that setting is determined by the `go` directive in the main module's `go.mod`. This means that building with a Go 1.26 compiler while declaring `go 1.24` in `go.mod` will produce a binary that behaves like Go 1.24 for any GODEBUG-controlled changes, while still receiving the compiler and standard library security improvements from Go 1.26.

The nightly test suite that runs in the operator repository provides additional coverage against compatibility regressions. If a go-toolset update introduces a behavioral change that affects the operator despite the compatibility promise and GODEBUG protection, the nightly tests are expected to surface the issue before it reaches a release.

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
cosign download sbom --platform=linux/amd64 quay.io/rhdh/rhdh-rhel9-operator:<tag> 2>/dev/null | \
  jq -r '.packages[] | select(.name == "stdlib") | .versionInfo'
```

Replace `<tag>` with the operator image tag (for example, `1.8` or `1.8-91`).

To check available go-toolset versions and determine when a new Go version has become available for the operator, browse the [Go Toolset for UBI 9](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690) page in the Red Hat Ecosystem Catalog. The catalog lists all available tags and their associated Go versions. Alternatively, query the registry directly:

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
- [UBI Content Availability and Life Cycle Policy](https://access.redhat.com/support/policy/updates/ubi) — UBI update cadence and support lifecycle
- [Go Toolset for UBI 9 — Red Hat Ecosystem Catalog](https://catalog.redhat.com/en/software/containers/ubi9/go-toolset/61e5c00b4ec9945c18787690) — available go-toolset image tags and metadata
- [UBI 9 Minimal — Red Hat Ecosystem Catalog](https://catalog.redhat.com/en/software/containers/ubi9/ubi-minimal/61832888c0d15aff4912fe0d) — the runtime base image
- [RHIDP-14096](https://redhat.atlassian.net/browse/RHIDP-14096) — the Jira issue that prompted this policy document