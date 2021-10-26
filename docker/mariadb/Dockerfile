FROM mariadb:10.6

# Configure golang environment to run migration against database
ARG TARGETARCH
RUN apt-get update && apt-get install -y curl && apt-get clean
RUN curl -L https://golang.org/dl/go1.16.2.linux-$TARGETARCH.tar.gz | tar xzf -
ENV GOROOT=/go
ENV PATH=$PATH:/go/bin

RUN mkdir /home/mysql && chown mysql:mysql /home/mysql

USER mysql

COPY --chown=mysql:mysql . /home/mysql/contest

WORKDIR /home/mysql

# Pre-build the migration script, make sure it builds.
RUN cd /home/mysql/contest && \
    go build github.com/facebookincubator/contest/tools/migration/rdbms

# All scripts in docker-entrypoint-initdb.d/ are automatically
# executed during container startup
COPY docker/mariadb/initdb.sql /docker-entrypoint-initdb.d/
COPY db/rdbms/schema/v0/create_contest_db.sql /

# Run all known migrations at the time of the creation of the container.
# From container documentation:
# """
# When a container is started for the first time, a new database with the
# specified name will be created and initialized with the provided configuration
# variables. Furthermore, it will execute files with extensions .sh, .sql and .sql.gz
# that are found in /docker-entrypoint-initdb.d. Files will be executed in alphabetical
# order.
# """
COPY docker/mariadb/migration.sh /docker-entrypoint-initdb.d/
