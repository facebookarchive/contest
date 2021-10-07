FROM golang:1.16.2-buster

RUN apt-get update && apt-get install -y mariadb-client openssh-server

# setup sshd for some plugin tests
RUN ssh-keygen -q -t rsa -f /root/.ssh/id_rsa -N ""
RUN cat /root/.ssh/id_rsa.pub > /root/.ssh/authorized_keys

WORKDIR ${GOPATH}/src/github.com/facebookincubator/contest
COPY  . .
RUN go get -t -v ./...
RUN chmod a+x docker/contest/tests.sh
