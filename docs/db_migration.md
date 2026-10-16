# Local database migration

Backstage stores plugin data in several PostgreSQL databases, usually named `backstage_plugin_<plugin>`. This guide covers upgrading the Operator-managed local database from PostgreSQL 15 to 18, or moving its databases to an external service such as AWS RDS or Azure Database for PostgreSQL.

The commands assume one Backstage instance with the default database resources and a Backstage Deployment. Multiple instances or other services using the same database require coordinated downtime for all clients.

## Preparation

Use `kubectl` or the equivalent `oc` commands. Set the namespace and Backstage CR name:

```bash
NAMESPACE=<your-namespace>
BACKSTAGE_CR=<backstage-cr-name>
DB_STATEFULSET="backstage-psql-${BACKSTAGE_CR}"
DB_POD="${DB_STATEFULSET}-0"
DB_PVC="data-${DB_POD}"
```

The database credentials are in `backstage-psql-secret-<backstage-cr-name>`. Keep this Secret throughout the upgrade, together with a copy of the Backstage CR and the database backup.

### Stop Backstage

Record the original replica count, then stop Backstage through the CR so reconciliation keeps it stopped while you copy or restore data:

```bash
BACKSTAGE_REPLICAS=$(kubectl -n "$NAMESPACE" get deployment "backstage-${BACKSTAGE_CR}" -o jsonpath='{.spec.replicas}')
kubectl -n "$NAMESPACE" get backstage "$BACKSTAGE_CR" -o yaml > backstage-before-migration.yaml
kubectl -n "$NAMESPACE" patch backstage "$BACKSTAGE_CR" --type merge \
  -p '{"spec":{"deployment":{"patch":{"spec":{"replicas":0}}}}}'
kubectl -n "$NAMESPACE" wait --for=delete pod \
  -l "rhdh.redhat.com/app=backstage-${BACKSTAGE_CR}" --timeout=5m
```

Keep Backstage and any other database clients stopped until the migration is verified.

## Upgrade PostgreSQL 15 to 18

RHDH 1.10 uses `registry.redhat.io/rhel9/postgresql-15`; RHDH 2.1 uses `registry.redhat.io/rhel10/postgresql-18`. This procedure follows PostgreSQL's [Upgrading Data via pg_dumpall](https://www.postgresql.org/docs/current/upgrading.html#UPGRADING-VIA-PGDUMPALL): dump the old databases, initialize a fresh database cluster, and restore the logical backup.

In disconnected installations, take the backup **before applying the 2.1 `rhdh-idms` ImageDigestMirrorSet**. The generated manifest replaces the 1.10 mirror configuration. Preserve the PostgreSQL 15 source-to-mirror mapping alongside the new PostgreSQL 18 mapping, and keep the mirrored PostgreSQL 15 image until the upgrade is verified.

1. Complete [Preparation](#preparation), including stopping Backstage. Check that the source server reports version `15.x`, and save the database list and locale information for comparison after restore:

   ```bash
   kubectl -n "$NAMESPACE" exec "$DB_POD" -- \
     psql -X -U postgres -d postgres -c 'SHOW server_version;' -c '\l'
   ```

2. Dump all databases and roles to a file on your local machine. Confirm the command succeeds and the file is nonempty before continuing:

   ```bash
   kubectl -n "$NAMESPACE" exec "$DB_POD" -- \
     pg_dumpall -U postgres > postgresql-15.sql
   test -s postgresql-15.sql
   ```

3. Stop the database:

   ```bash
   kubectl -n "$NAMESPACE" scale statefulset "$DB_STATEFULSET" --replicas=0
   kubectl -n "$NAMESPACE" wait --for=delete "pod/$DB_POD" --timeout=5m
   ```

4. Upgrade the Operator from RHDH 1.10 to 2.1 using your installation's normal upgrade procedure. Wait for the upgraded Operator to become ready and reconcile the database StatefulSet. Verify that the StatefulSet still has zero replicas and now selects the PostgreSQL 18 image:

   ```bash
   kubectl -n "$NAMESPACE" get statefulset "$DB_STATEFULSET" \
     -o jsonpath='{.spec.replicas}{" "}{.spec.template.spec.containers[0].image}{"\n"}'
   ```

   The default database template leaves the replica count under your control during this procedure.

5. Replace the old data volume and start PostgreSQL 18. Deleting this PVC removes the PostgreSQL 15 data; keep the successful dump and the existing credentials Secret:

   ```bash
   kubectl -n "$NAMESPACE" delete pvc "$DB_PVC" --wait=true
   kubectl -n "$NAMESPACE" scale statefulset "$DB_STATEFULSET" --replicas=1
   kubectl -n "$NAMESPACE" rollout status "statefulset/$DB_STATEFULSET" --timeout=10m
   kubectl -n "$NAMESPACE" exec "$DB_POD" -- \
     psql -X -U postgres -d postgres -c 'SHOW server_version;'
   ```

   The server must report version `18.x` before you restore the backup.

6. Restore the dump into the fresh database cluster:

   ```bash
   kubectl -n "$NAMESPACE" exec -i "$DB_POD" -- \
     psql -X -U postgres -d postgres < postgresql-15.sql
   ```

   The only expected error is `ERROR: role "postgres" already exists`, because the new cluster already has this bootstrap role. Review the restore output for any other errors before proceeding; `psql` can finish successfully despite SQL errors.

7. List the restored databases and compare their names, encodings, and locales with the source. Check important application data as well:

   ```bash
   kubectl -n "$NAMESPACE" exec "$DB_POD" -- \
     psql -X -U postgres -d postgres -c '\l'
   ```

   Logical restore rebuilds indexes using the target image's collation libraries and records their current versions, clearing collation-version warnings associated with the old volume. The following check should return no rows; an unconditional collation-version refresh is unnecessary:

   ```bash
   kubectl -n "$NAMESPACE" exec "$DB_POD" -- psql -X -U postgres -d postgres -c \
     'SELECT datname FROM pg_database
      WHERE datallowconn
        AND datcollversion IS DISTINCT FROM pg_database_collation_actual_version(oid);'
   ```

8. [Restart Backstage](#restart-backstage). Retain the backup until application verification is complete.

## Move databases to an external server

Complete [Preparation](#preparation), including stopping Backstage. You can copy databases individually with [pg_dump](https://www.postgresql.org/docs/current/app-pgdump.html) and [psql](https://www.postgresql.org/docs/current/app-psql.html), or adapt [db_copy.sh](../hack/db_copy.sh).

### Prerequisites

- Install compatible `pg_dump` and `psql` clients locally.
- The source database user needs privileges to dump all application databases.
- The destination database user needs privileges to create databases and restore their contents.
- Configure source authentication, for example with a PostgreSQL password file.

### Forward the source database port

Run this in a separate terminal, using the same namespace and Pod name as above:

```bash
kubectl -n <your-namespace> port-forward <database-pod-name> 15432:5432
```

Keep this terminal running until the copy is complete. The source is then available at `127.0.0.1:15432`.

### Configure the copy

Copy `db_copy.sh` and set `to_host`, `to_port`, and `to_user` for the destination, and `from_host`, `from_port`, and `from_user` for the forwarded source.

Replace the script's example `allDB` array with **every application database** in your instance, including custom database names. Discover the databases instead of assuming the six names in the script are complete:

```bash
psql -h 127.0.0.1 -p 15432 -U postgres -d postgres -Atc \
  "SELECT datname FROM pg_database WHERE NOT datistemplate AND datname <> 'postgres' ORDER BY datname;"
```

Put each returned name in the `allDB` array as a separate quoted entry. The query excludes the maintenance database `postgres`; if you store application data there, copy that data separately with `pg_dump` and `psql`. The script creates its destination databases, so those databases must not already exist.

### Copy the data

```bash
TO_PSW=<destination-db-password> /bin/bash /path/to/db_copy.sh
```

Review the dump and restore output, then compare the source and destination databases and application data. For large databases, consider the [documented compression options](https://www.postgresql.org/docs/current/backup-dump.html#BACKUP-DUMP-LARGE). Stop port forwarding after the copy completes.

### Reconfigure the Backstage CR

Follow [External DB integration](external-db.md) to create the external connection and certificate Secrets. Add them to the CR while retaining the zero-replica patch:

```yaml
spec:
  database:
    enableLocalDb: false
  application:
    extraFiles:
      secrets:
        - name: <crt-secret>
          key: postgres-crt.pem
    extraEnvs:
      secrets:
        - name: <cred-secret>
```

Apply these changes, then [restart Backstage](#restart-backstage) and verify it uses the external database.

### Clean up the local database

Setting `spec.database.enableLocalDb: false` retains the old StatefulSet, Pod, and other local database resources, as described in the [Resource Deletion Policy](admin.md#resource-deletion-policy). After verifying the external database and retaining a backup, remove the unused StatefulSet and PVC manually:

```bash
kubectl -n "$NAMESPACE" delete statefulset "$DB_STATEFULSET"
kubectl -n "$NAMESPACE" delete pvc "$DB_PVC"
```

## Restart Backstage

Restore the original replica count and wait for the application to become ready:

```bash
kubectl -n "$NAMESPACE" patch backstage "$BACKSTAGE_CR" --type merge \
  -p "{\"spec\":{\"deployment\":{\"patch\":{\"spec\":{\"replicas\":${BACKSTAGE_REPLICAS}}}}}}"
kubectl -n "$NAMESPACE" wait "deployment/backstage-${BACKSTAGE_CR}" \
  --for="jsonpath={.spec.replicas}=${BACKSTAGE_REPLICAS}" --timeout=5m
kubectl -n "$NAMESPACE" rollout status "deployment/backstage-${BACKSTAGE_CR}" --timeout=15m
```

Verify that Backstage starts successfully and the expected plugin data is available.

## Troubleshooting

### Collation version mismatch after a PostgreSQL image update

Reusing an existing data directory after an image's operating system or collation libraries change can produce collation-version mismatch warnings. Unlike logical restore into a fresh cluster, updating version metadata alone does not rebuild indexes.

[Stop Backstage](#stop-backstage) and rebuild the indexes:

```bash
kubectl -n "$NAMESPACE" exec "$DB_POD" -- reindexdb -U postgres --all
```

Only after reindexing succeeds, refresh the recorded version for each database returned by the mismatch check:

```bash
kubectl -n "$NAMESPACE" exec "$DB_POD" -- psql -X -U postgres -d postgres -v ON_ERROR_STOP=1 -c \
  'ALTER DATABASE "<database>" REFRESH COLLATION VERSION;'
```

Verify the mismatch check above returns no rows, then [restart Backstage](#restart-backstage). For explicitly defined collations and other dependent objects, follow PostgreSQL's [ALTER COLLATION notes](https://www.postgresql.org/docs/current/sql-altercollation.html#SQL-ALTERCOLLATION-NOTES).

### Migration table is already locked

If Backstage fails with `Can't take lock to run migrations: Migration table is already locked`, stop Backstage and confirm no migrations are running before releasing the lock in the affected plugin database:

```bash
psql -h <to_host> -U <to_user> -d <database> -c 'DELETE FROM knex_migrations_lock;'
```

For example, the `auth` plugin uses `backstage_plugin_auth`. Some plugins use a different lock table, such as `backstage_backend_tasks__knex_migrations_lock` in `backstage_plugin_search`.
