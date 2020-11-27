#!/bin/bash
set -eo pipefail
echo "Running database migrations..."
source ${HOME}/.gvm/scripts/gvm 
gvm use 1.13
cd ${GOPATH}/src/github.com/facebookincubator/contest
# Migrate main db and integration tests db
go run tools/migration/rdbms/main.go -dbURI "contest:contest@unix(/var/run/mysqld/mysqld.sock)/contest?parseTime=true" -dir db/rdbms/migration up
go run tools/migration/rdbms/main.go -dbURI "contest:contest@unix(/var/run/mysqld/mysqld.sock)/contest_integ?parseTime=true" -dir db/rdbms/migration up

