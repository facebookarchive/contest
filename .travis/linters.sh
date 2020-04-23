#!/usr/bin/env bash
set -exu

GO111MODULE=on
# installing golangci-lint as recommended on the project page
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh | sh -s -- -b $(go env GOPATH)/bin latest
cd "${TRAVIS_BUILD_DIR}"
go mod download
golangci-lint run --disable typecheck --enable deadcode --enable varcheck --enable staticcheck

# check license headers
# this needs to be run from the top level directory, because it uses
# `git ls-files` under the hood.
go get -u github.com/u-root/u-root/tools/checklicenses
go install github.com/u-root/u-root/tools/checklicenses
cd "${TRAVIS_BUILD_DIR}"
echo "[*] Running checklicenses"
go run github.com/u-root/u-root/tools/checklicenses -c .travis/config.json
