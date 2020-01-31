FROM golang:1.13.0-buster

RUN apt-get update && apt-get install -y mariadb-client


WORKDIR ${GOPATH}/src/github.com/facebookincubator/contest
COPY  . .
RUN go get -t -v ./...
RUN chmod a+x docker/contest/tests.sh
