#!/usr/bin/env bash

# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

# because things are never simple.
# See https://github.com/codecov/example-go#caveat-multiple-files
# and https://github.com/insomniacslk/dhcp/tree/master/.travis/tests.sh

set -e

# Wait until mysql instance is up and running.
attempts=0
max_attempts=5
while true; do
  echo "Waiting for mysql to settle"
  mysqladmin -h mysql -P 3306 -u contest --protocol tcp --password=contest ping && break || true
  if test ${attempts} -eq ${max_attempts}; then
    echo "MySQL is not healthy after ${max_attempts} attempts"
    exit 1
  fi
  let attempts=${attempts}+1
  echo "MySQL is not healthy, retrying in 5s"
  sleep 5
done

echo "MySQL is healthy!"

# disable CGO for the build
export CGO_ENABLED=0
for d in $(go list ./cmds/... | grep -v vendor); do
    go build "${d}"
done

# CGO required for the race detector
export CGO_ENABLED=1
echo "" > coverage.txt

for d in $(go list ./... | grep -v vendor); do
    go test -race -coverprofile=profile.out -covermode=atomic "${d}"
    if [ -f profile.out ]; then
      cat profile.out >> coverage.txt
      rm profile.out
    fi
done

echo "Running integration tests"
for d in $(go list -tags="integration integration_storage" ./... | grep integ | grep -Ev "integ$|common$|vendor"); do
    # integration tests
    echo "Running integration tests for ${d}"
    go test -c -tags="integration" -race -coverprofile=profile.out -covermode=atomic "${d}"
    testbin="./$(basename "${d}").test"
    test -x "${testbin}" && ./"${testbin}" -test.v
    if [ -f profile.out ]; then
      cat profile.out >> coverage.txt
      rm profile.out
    fi
    rm $testbin

    # Storage tests are split across TestSuites in multiple packages. Within a TestSuite,
    # tests do not run in parallel, but tests in different packages might run in parallel
    # according to GOMAXPROCS. Storage tests are not safe to run in parallel as they 
    # make assertions on the data that is persisted in the database. Therefore, use "-p1"
    # to have tests run serially.
    echo "Running integration tests with storage layer for ${d}"
    go test -c -tags="integration_storage" -p 1 -race -coverprofile=profile.out -covermode=atomic "${d}"
    testbin="./$(basename "${d}").test"
    # only run it if it was built - i.e. if there are integ tests
    test -x "${testbin}" && ./"${testbin}" -test.v
    if [ -f profile.out ]; then
      cat profile.out >> coverage.txt
      rm profile.out
    fi
    rm $testbin
done
