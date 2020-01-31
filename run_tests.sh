#!/bin/bash

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
