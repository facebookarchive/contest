#!/usr/bin/env bash
#
# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

set -eu

export CI=${CI:-false}

if [ "${UID}" -ne 0 ]
then
    echo "Re-running as root with sudo"
    sudo --preserve-env=CI,GITHUB_ACTIONS,GITHUB_REF,GITHUB_REPOSITORY,GITHUB_HEAD_REF,GITHUB_SHA,GITHUB_RUN_ID,PATH "$0" "$@"
    exit $?
fi

codecov_env=`bash <(curl -s https://codecov.io/env)`
docker-compose build mysql contest
docker-compose run \
    ${codecov_env} -e "CI=${CI}" \
    contest \
    /go/src/github.com/facebookincubator/contest/docker/contest/tests.sh \
