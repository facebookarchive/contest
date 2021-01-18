#!/bin/bash
set -eo pipefail
echo "Running database migrations..."
cd /home/mysql/contest
# Migrate main db and integration tests db
go run tools/migration/rdbms/main.go -dbURI "contest:contest@unix(/var/run/mysqld/mysqld.sock)/contest?parseTime=true" -dir db/rdbms/migration up
go run tools/migration/rdbms/main.go -dbURI "contest:contest@unix(/var/run/mysqld/mysqld.sock)/contest_integ?parseTime=true" -dir db/rdbms/migration up

