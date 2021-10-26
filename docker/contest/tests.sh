#!/usr/bin/env bash
#
# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

# because things are never simple.
# See https://github.com/codecov/example-go#caveat-multiple-files
# and https://github.com/insomniacslk/dhcp/tree/master/.travis/tests.sh

set -eu

CI=${CI:-false}

for d in $(go list ./cmds/... | grep -v vendor); do
    CGO_ENABLED=0 go build "${d}"
done

i=1
for d in $(go list ./... | grep -v vendor); do
    # Run in stress mode first
    go test -race -count=9 -failfast "${d}"
    # Then in coverage mode
    go test -race -coverprofile="unit.$i.cov" -covermode=atomic "${d}"
    i=$(($i + 1))
done

# MySQL should be up by now.
if ! mysqladmin -h dbstorage -P 3306 -u contest --protocol tcp --password=contest ping; then
    echo "MySQL is not ready!"
    exit 1
fi

# Run integration tests collecting coverage only for the business logic (pkg directory)
i=1
for tag in integration integration_storage; do
    echo "Running integration tests with tag \"${tag}\""
    for d in $(go list -tags=${tag} ./tests/... | grep -Ev "integ$|common$|vendor"); do
        pflag=""
        if test ${tag} = "integration_storage"; then
          # Storage tests are split across TestSuites in multiple packages. Within a TestSuite,
          # tests do not run in parallel, but tests in different packages might run in parallel
          # according to GOMAXPROCS. Storage tests are not safe to run in parallel as they
          # make assertions on the data that is persisted in the database. Therefore, use "-p1"
          # to have tests run serially.
          pflag="-p 1"
        fi
        go test -tags=${tag} -race -count=4 -failfast ${pflag} "${d}"
        go test -tags=${tag} -race \
          -coverprofile="integ.$i.cov" ${pflag} \
          -covermode=atomic \
          -coverpkg=all \
          "${d}"
        i=$(($i + 1))
    done
done

# Run E2E tests.
# They are run explicitly to avoid unintended influence of global variables.
i=1
echo "Running E2E tests"
for t in TestE2E/TestCLI TestE2E/TestSimple TestE2E/TestPauseResume; do
    echo "  $t"
    go test -tags=e2e -race -coverprofile="e2e.$i.cov" -covermode=atomic -coverpkg=all ./tests/e2e/... -run "$t"
    i=$(($i + 1))
done

if [ "${CI}" == "true" ]
then
    bash <(curl -s https://codecov.io/bash) -c -f "./unit.*.cov" -F unittests
    bash <(curl -s https://codecov.io/bash) -c -f "./integ.*.cov" -F integration
    bash <(curl -s https://codecov.io/bash) -c -f "./e2e.*.cov" -F e2e
else
    echo "Skipping upload of coverage profiles because not running in a CI"
fi
