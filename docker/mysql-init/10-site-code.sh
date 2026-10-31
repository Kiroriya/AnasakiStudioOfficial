#!/bin/sh
# Anasaki Studio — creates the Site Code database and its MySQL user.
#
# The official mysql image runs the files in /docker-entrypoint-initdb.d/ once, when the data
# volume is empty (first start). Existing installs: run the four statements shown in the admin
# (Site code → Database) or in docs/site-code.md, then set CODE_DB_DSN.
#
#   <MYSQL_DATABASE>_code        the database of the site's own tables (+ read-only core_* views)
#   <MYSQL_DATABASE>_code user   ALL on that database only — it cannot see the CMS tables
#   <MYSQL_USER>                 also ALL on it (creates the core_* views, backups, admin screens)
#
# Reads MYSQL_DATABASE, MYSQL_USER, MYSQL_ROOT_PASSWORD and CODE_DB_PASSWORD from the container.

if [ -z "${CODE_DB_PASSWORD:-}" ]; then
  echo "10-site-code.sh: CODE_DB_PASSWORD is not set — Site Code database not created" >&2
  exit 1
fi

code_db="${MYSQL_DATABASE:-anasaki}_code"
code_user="${code_db}"
cms_user="${MYSQL_USER:-anasaki}"

# single quotes doubled for SQL string literals
sql_str() { printf "%s" "$1" | sed "s/'/''/g"; }

echo "10-site-code.sh: creating database ${code_db} and user ${code_user}"
mysql --protocol=socket -uroot -p"${MYSQL_ROOT_PASSWORD}" <<EOSQL
CREATE DATABASE IF NOT EXISTS \`${code_db}\` CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
CREATE USER IF NOT EXISTS '$(sql_str "$code_user")'@'%' IDENTIFIED BY '$(sql_str "$CODE_DB_PASSWORD")';
GRANT ALL PRIVILEGES ON \`${code_db}\`.* TO '$(sql_str "$code_user")'@'%';
GRANT ALL PRIVILEGES ON \`${code_db}\`.* TO '$(sql_str "$cms_user")'@'%';
FLUSH PRIVILEGES;
EOSQL
