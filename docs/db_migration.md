## Move Backstage databases to external DB server

By default, Backstage hosts data for each plugin, so there are usually several databases, depending on the number of plugins, those databases usually prefixed with "backstage_plugin_".

````
postgres=> \l
List of databases
Name                        |  Owner   | Encoding | Locale Provider |   Collate   |    Ctype    |   
----------------------------+----------+----------+-----------------+-------------+-------------+
backstage_plugin_app        | postgres | UTF8     | libc            | en_US.UTF-8 | en_US.UTF-8 | 
backstage_plugin_auth       | postgres | UTF8     | libc            | en_US.UTF-8 | en_US.UTF-8 |
backstage_plugin_catalog    | postgres | UTF8     | libc            | en_US.UTF-8 | en_US.UTF-8 |
backstage_plugin_permission | postgres | UTF8     | libc            | en_US.UTF-8 | en_US.UTF-8 |
backstage_plugin_scaffolder | postgres | UTF8     | libc            | en_US.UTF-8 | en_US.UTF-8 |
backstage_plugin_search     | postgres | UTF8     | libc            | en_US.UTF-8 | en_US.UTF-8 |
postgres                    | postgres | UTF8     | libc            | en_US.UTF-8 | en_US.UTF-8 |
````

To move the data from working Backstage instance hosted on a local PostgreSQL server to a production-ready PostgreSQL service (such as AWS RDB or Azure Database), you can use directly PostgreSQL utilities such as [pg_dump](https://www.postgresql.org/docs/current/app-pgdump.html) with [psql](https://www.postgresql.org/docs/current/app-psql.html) or [pgAdmin](https://www.pgadmin.org/) and move the data from each database one-by-one.
To simplify this process, we have a [**db_copy.sh**](../hack/db_copy.sh) script.

### Prerequisites

- [**pg_dump**](https://www.postgresql.org/docs/current/backup-dump.html) and [**psql**](https://www.postgresql.org/docs/current/app-psql.html) client utilities installed on your local machine.
- For data export the **PGSQL user** sufficient privileges to make a full dump of source (local) databases 
- For data import the **PGSQL user** sufficient admin privileges to create (external) databases and populate it with database dumps
 

### Make [port forwarding](https://kubernetes.io/docs/tasks/access-application-cluster/port-forward-access-application-cluster/) of the source (local) database pod. 

````
kubectl port-forward -n <your-namespace> <pgsql-pod-name> <forward-to-port>:<forward-from-port>
````

Where:

- **pgsql-pod-name**  a name of PostgreSQL pod with format like backstage-psql-<backstage-cr-name>-<_index>
- **forward-to-port** port of your choice to forward PGSQL to
- **forward-from-port** PGSQL port, usually 5432

For example:

````
kubectl port-forward -n backstage backstage-psql-backstage1-0 15432:5432
Forwarding from 127.0.0.1:15432 -> 5432
Forwarding from [::1]:15432 -> 5432
````
**NOTE:** it has to be run on a dedicated terminal and interrupted as soon as data copy is completed.

### Configure PGSQL connection

Make a copy of **db_copy.sh** script and modify it according to your configuration:

* **to_host**=destination host name (e g #.#.#.rds.amazonaws.com)
* **to_port**=destination port (usually 5432) 
* **to_user**=destination server username (e g postgres)
* **from_host**=usually 127.0.0.1
* **from_port**=< forward-to-port >
* **from_user**=source server username (e g postgres)
* **allDB**=name of databases for import in double quotes separated by spaces, e g  ("backstage_plugin_app" "backstage_plugin_auth" "backstage_plugin_catalog" "backstage_plugin_permission" "backstage_plugin_scaffolder" "backstage_plugin_search")

### Create destination databases and copy data

````
/bin/bash TO_PSW=<destination-db-password> /path/to/db_copy.sh
````

It will produce some output about **pg_dump** and **psql** progressing.
When successfully finished you can stop port forwarding.

**NOTE:** In case if your databases are quite big already, you may consider using compression tools as [documented](https://www.postgresql.org/docs/current/backup-dump.html#BACKUP-DUMP-LARGE)

### Reconfigure Backstage Custom Resource

Reconfigure Backstage according to [External DB configuration](external-db.md) i.e.
* Create external DB connection Secret
* Create a Secret with certificate
* Configure CR disabling local DB and adding those 2 Secrets as extraEnv and extraFile accordingly.

At the end your Backstage.spec CR should contain the following:
````
spec:
 database:
   enableLocalDb: false 
 application:
 ... 
   extraFiles:
     secrets:
       - name: <crt-secret> 
         key: postgres-crt.pem # key name as in <crt-secret> Secret
   extraEnvs:
     secrets:
        - name: <cred-secret> 
 ...        
````
Apply these changes.

### Clean local Persistence Volume

When Backstage is reconfigured with **spec.database.enableLocalDb: false** it deletes corresponding StatefulSet and Pod(s) but Persistence Volume Claim and associated Persistence Volume retained.
You need to clean it manually with

````
 kubectl -n backstage delete pvc <local-psql-pvc-name>
````

Where **local-psql-pvc-name** has format **data-<psql-pod-name**>  (see above)


### Troubleshooting

Backstage container may fail with Crash Loop Backoff error and "Can't take lock to run migrations: Migration table is already locked" log error:

````
2024-06-02T20:37:44.941Z catalog info Performing database migration                                                                                                                                               │
Can't take lock to run migrations: Migration table is already locked                                                                                                                                              │
If you are sure migrations are not running you can release the lock manually by running 'knex migrate:unlock'                                                                                                     │
/opt/app-root/src/node_modules/@backstage/backend-app-api/dist/index.cjs.js:1793                                                                                                                                  │
          throw new errors.ForwardedError(                                                                                                                                                                        │
                ^                                                                                                                                                                                                 │
 MigrationLocked: Plugin 'auth' startup failed; caused by MigrationLocked: Migration table is already locked                                                                                                       │
````                       

A way to make it work without knex utility is to delete the data from the **knex_migrations_lock** table for each problematic Backstage plugin's database (in the example above it is **'auth'** plugin, so corresponding database is **backstage_plugin_auth**):

````
psql -h <to_host> -U <to_user> -d <database> -c "delete from knex_migrations_lock;"
````

**NOTE:** in some DB the table is called differently like: **backstage_backend_tasks__knex_migrations_lock** in the **backstage_plugin_search** database




 
