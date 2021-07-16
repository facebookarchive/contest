#!/usr/bin/env bash
#
# Copyright (c) Facebook, Inc. and its affiliates.
#
# This source code is licensed under the MIT license found in the
# LICENSE file in the root directory of this source tree.

set -eu

export DATABASE=${database:-mysql}
export CI=${CI:-false}
export DOCKER_BUILDKIT=1

while getopts d: flag
do
    case "${flag}" in
        d) DATABASE=${OPTARG};;
    esac
done

if [ "${UID}" -ne 0 ]
then
    echo "Re-running as root with sudo"
    sudo --preserve-env=DATABASE,CI,GITHUB_ACTIONS,GITHUB_REF,GITHUB_REPOSITORY,GITHUB_HEAD_REF,GITHUB_SHA,GITHUB_RUN_ID,PATH "$0" "$@"
    exit $?
fi

COMPOSE_FILE=docker-compose.yml
if [ "${DATABASE}" != "mysql" ]
then
    COMPOSE_FILE=docker-compose.$DATABASE.yml
fi

codecov_env=`bash <(curl -s https://codecov.io/env)`
docker-compose -f $COMPOSE_FILE build $DATABASE contest
docker-compose -f $COMPOSE_FILE run \
    ${codecov_env} -e "CI=${CI}" \
    contest \
    /go/src/github.com/facebookincubator/contest/docker/contest/tests.sh \
