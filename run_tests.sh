#!/bin/bash
# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

set -eu
if [ "${UID}" -ne 0 ]
then
    echo "Re-running as root with sudo"
    sudo "$0" "$@"
    exit $?
fi

codecov_env=`bash <(curl -s https://codecov.io/env)`
docker-compose build mysql contest
docker-compose run \
    ${codecov_env} \
    contest \
    /go/src/github.com/facebookincubator/contest/docker/contest/tests.sh \
