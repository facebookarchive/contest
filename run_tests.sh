#!/bin/bash
# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

if [ "${UID}" -ne 0 ]
then
    echo "Re-running as root with sudo"
    sudo "$0" "$@"
    exit $?
fi

docker-compose \
    -f docker-compose.yml \
    -f docker-compose.tests.yml \
    up \
    --build \
    --exit-code-from contest
