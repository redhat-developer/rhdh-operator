# PostgreSQL 15 to 18 upgrade documentation plan

Tracked by [RHIDP-14594](https://redhat.atlassian.net/browse/RHIDP-14594).

## Recommended approach

Add an **Operator-specific PostgreSQL 15 → 18 upgrade guide for a single deployed RHDH instance**, covering the **RHDH 1.10 → 2.1** transition and based on the existing dump-and-restore E2E flow. Link to PostgreSQL's documentation for database fundamentals, and provide the concrete Operator/Kubernetes steps needed to perform the migration.

### Findings that shape the guide

- [RHIDP-14594](https://redhat.atlassian.net/browse/RHIDP-14594) requires a documented procedure, E2E validation, contributions to both Operator and chart documentation, and downstream-documentation evaluation.
- [Operator PR #3569](https://github.com/redhat-developer/rhdh-operator/pull/3569) supplies the implemented migration flow. The current test verifies data in `backstage_plugin_catalog`, database names, replacement Pod/PVC identities, preservation of the credentials Secret, and Backstage recovery.
- [docs/db_migration.md](db_migration.md) describes moving to an **external database**. A dedicated guide will make this major-version upgrade easier to find.
- `RELATED_IMAGE_postgresql` overrides the database image, including an image supplied through raw runtime configuration. An Operator upgrade can therefore select PG18 while the existing PVC still contains PG15 data.
- `rhdh.redhat.com/idle=true` stops **both Backstage and PostgreSQL**. The migration must instead stop Backstage through its CR deployment patch while PostgreSQL remains available for the dump.
- The documented Fedora migration uses logical dump/restore directly from 15 to 18. The earlier RHEL `POSTGRESQL_UPGRADE=copy` experiments and RHDH Local procedure use a different mechanism.
- The migration-helper PR mentioned in the Jira comment converts **Helm values**; it does not migrate PostgreSQL data.

## Implementation plan

### 1. Establish the supported procedure and installation variants

Use these as the primary references:

- [tests/e2e/postgresql_upgrade_test.go](../tests/e2e/postgresql_upgrade_test.go)
- [tests/e2e/e2e_upgrade_test.go](../tests/e2e/e2e_upgrade_test.go)
- [Earlier Operator migration experiment, rhdh #5145](https://github.com/redhat-developer/rhdh/pull/5145)
- [PostgreSQL 18 upgrade documentation](https://www.postgresql.org/docs/18/upgrading.html#UPGRADING-VIA-PGDUMPALL)
- [The sclorg container documentation](https://github.com/sclorg/postgresql-container/blob/master/src/root/usr/share/container-scripts/postgresql/README.md) for image-specific behaviour.

Define the baseline as **one deployed RHDH instance with an Operator-managed local PG15 database, upgrading from `release-1.10` to `release-2.1` and migrating to PG18**. Parameterize namespace, CR name, Operator Deployment, and target image; discover actual PVC and Secret references to accommodate customized installations.

Include only a brief note for multiple-instance installations: **installations with multiple RHDH instances managed by the same Operator require different coordination steps; this procedure covers a single instance**.

Cover direct installations and OLM installations, with installation-specific image configuration and upgrade coordination. **OLM suspension/resumption needs a separate rehearsal:** the existing upgrade E2E explicitly supports direct deployment mode only.

### 2. Write `docs/postgresql-upgrade.md`

Organize it as a short explanation followed by a numbered runbook:

| Phase | Content |
|---|---|
| **Prepare** | Confirm the single-instance baseline and the RHDH 1.10 → 2.1 transition. Identify images, resources, credentials Secret, original replica configuration, and application-data checks. Explain downtime. |
| **Stop writers** | Set Backstage replicas to zero through `spec.deployment.patch.spec.replicas`; wait for all application Pods to terminate. Address autoscaling/GitOps and additional database consumers, including Orchestrator when applicable. |
| **Back up** | Run `pg_dumpall`, check command success and a non-empty dump, and retain the backup securely outside the database volume. Explain PostgreSQL's preference for a newer dump client and the source-client approach exercised by E2E. |
| **Coordinate cutover** | Select the PG18 image through the appropriate installation configuration. Suspend reconciliation for volume replacement, with explicit stop/resume checks. |
| **Provision PG18** | Remove the exact PostgreSQL StatefulSet and data PVC after backup verification. Preserve the Backstage CR and credentials Secret. Resume reconciliation and verify a fresh PG18 database. |
| **Restore** | Restore using the target `psql`, capture output, and check for SQL errors. Explain expected bootstrap-object conflicts rather than broadly ignoring “already exists” errors. |
| **Verify and resume** | Compare databases and representative application data, verify Secret preservation and the running server version, then restore the original Backstage replica configuration and check application readiness. |

Include concise troubleshooting and recovery guidance:

- PG18 attempting to start against a PG15 data directory.
- PVC deletion/provisioning delays.
- Unexpected restore errors and partially restored databases.
- Credential mismatches.
- Collation-version warnings.
- Recovery from the retained source backup; switching an image back does not downgrade a PG18 data directory.

For collations, follow [PostgreSQL's guidance](https://www.postgresql.org/docs/18/sql-altercollation.html#SQL-ALTERCOLLATION-NOTES): **refreshing the recorded version does not rebuild indexes**. Explain that logical restoration rebuilds objects and link to the rebuilding requirements for any remaining mismatches.

### 3. Make the guide discoverable

| File | Planned change |
|---|---|
| `README.md` | Add the upgrade guide under “More documentation”. |
| `docs/configuration.md` | Link from “Database Configuration”, explaining that a major image change requires data migration. |
| `docs/admin.md` | Link from the administration guidance covering local databases/instance maintenance. |
| `docs/db_migration.md` | Add a cross-reference for readers upgrading the local database. |
| `tests/e2e/README.md` | Link the test scenario to the administrator procedure, document the `release-1.10` → `release-2.1` path, and summarize the assertions it validates. |

### 4. Constrain the PostgreSQL upgrade tests to the release transition

Exercise only **`release-1.10` → `release-2.1`** in the PostgreSQL upgrade scenario, matching the release transition that changes the default database from PG15 to PG18.

- Set the sole matrix entry in [`.github/workflows/nightly-postgresql-upgrade-test.yaml`](../.github/workflows/nightly-postgresql-upgrade-test.yaml) to:

  ```yaml
  from_branch: release-1.10
  to_branch: release-2.1
  ```

- Use the `release-1.10` source manifest and check out `release-2.1` to build the target Operator image, generate its manifest, and run its E2E suite.
- Update the command in `tests/e2e/README.md` to run from a `release-2.1` checkout with a target manifest generated from that branch.
- Retain the PG15 → PG18 server-version, application-data survival, credentials Secret preservation, Pod/PVC replacement, and Backstage recovery assertions.

### 5. Validate the documented commands

- Check shell syntax, relative links, resource names, and CR patch semantics.
- Execute the runbook verbatim on a disposable installation with one RHDH instance, using pinned `release-1.10` source and `release-2.1` target versions and images.
- Verify application-data survival, Secret preservation, actual PG18 server version, and restoration of the original replica settings.
- Rehearse the OLM-specific controls before publishing those commands.
- Run the PostgreSQL upgrade E2E for `release-1.10` → `release-2.1` and record the tested commit SHAs, image digests, and evidence for that exact branch pair.

There is historical successful [nightly evidence from October 2, 2026](https://github.com/redhat-developer/rhdh-operator/actions/runs/36949546080) for `release-1.10` → `main`. The latest nightly checked on October 13, 2026 [failed during the target image build](https://github.com/redhat-developer/rhdh-operator/actions/runs/38331285290), before migration. Fresh validation must use a reproducible `release-2.1` target image and provide evidence for the exact `release-1.10` → `release-2.1` path.

### 6. Align the deliverable with Jira

Document the validated procedure and evidence in the eventual PR. RHIDP-14594 also needs the corresponding chart contribution and downstream-documentation evaluation; identify those follow-ups when assessing ticket completion.
