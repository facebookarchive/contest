#!/usr/bin/env bash

# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

# because things are never simple.
# See https://github.com/codecov/example-go#caveat-multiple-files
# and https://github.com/insomniacslk/dhcp/tree/master/.travis/tests.sh

set -eu

CI=${CI:-false}

# Wait until mysql instance is up and running.
attempts=0
max_attempts=5
while true; do
  echo "Waiting for mysql to settle"
  mysqladmin -h localhost -P 3306 -u contest --protocol tcp --password=contest ping && break || true
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
      cat profile.out >> coverage_unittests.txt
      rm profile.out
    fi
done

# Run integration tests collecting coverage only for the business logic (pkg directory)
for tag in integration integration_storage; do
    echo "Running integration tests with tag \"${tag}\""
    for d in $(go list -tags=${tag} ./... | grep integ | grep -Ev "integ$|common$|vendor"); do
        pflag=""
        if test ${tag} = "integration_storage"; then
          # Storage tests are split across TestSuites in multiple packages. Within a TestSuite,
          # tests do not run in parallel, but tests in different packages might run in parallel
          # according to GOMAXPROCS. Storage tests are not safe to run in parallel as they
          # make assertions on the data that is persisted in the database. Therefore, use "-p1"
          # to have tests run serially.
          pflag="-p 1"
        fi
        go test -tags=${tag} -race \
          -coverprofile=profile.out ${pflag} \
          -covermode=atomic \
          -coverpkg=all \
          "${d}"
        if [ -f profile.out ]; then
          cat profile.out >> coverage_integration.txt
          rm profile.out
        fi
    done
done

if [ "${CI}" == "true" ]
then
    echo "Uploading coverage profile for unit tests"
    bash <(curl -s https://codecov.io/bash) -c -f coverage_unittests.txt -F unittests
    echo "Uploading coverage profile for integration tests"
    bash <(curl -s https://codecov.io/bash) -c -f coverage_integration.txt -F integration
    bash <(curl -s https://codecov.io/bash) -c -F integration_storage
else
    echo "Skipping upload of coverage profiles because not running in a CI"
fi
