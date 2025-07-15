#!/bin/bash

set -euo pipefail

: "${TO_PSW:?TO_PSW environment variable not set}"

to_host="<db-service-host>"
to_port=5432
to_user=postgres

from_host=127.0.0.1
from_port=15432
from_user=postgres

allDB=("backstage_plugin_app" "backstage_plugin_auth" "backstage_plugin_catalog" "backstage_plugin_permission" "backstage_plugin_scaffolder" "backstage_plugin_search")

for db in "${allDB[@]}"; do
  db=${allDB["$db"]}
  echo Copying database: "$db"
  PGPASSWORD="$TO_PSW" psql -h "$to_host" -p "$to_port" -U "$to_user" -c "create database \"$db\";"
  pg_dump -h "$from_host" -p "$from_port" -U "$from_user" -d "$db" | PGPASSWORD="$TO_PSW" psql -h "$to_host" -p "$to_port" -U "$to_user" -d "$db"
done