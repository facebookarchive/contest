#!/bin/bash
#
# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

set -eo pipefail
echo "Running database migrations..."
cd /home/mysql/contest
# Migrate main db and integration tests db
go run tools/migration/rdbms/main.go -dbURI "contest:contest@unix(/var/run/mysqld/mysqld.sock)/contest?parseTime=true" -dir db/rdbms/migration up
go run tools/migration/rdbms/main.go -dbURI "contest:contest@unix(/var/run/mysqld/mysqld.sock)/contest_integ?parseTime=true" -dir db/rdbms/migration up
