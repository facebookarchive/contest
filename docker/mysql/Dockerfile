FROM mysql

# All scripts in docker-entrypoint-initdb.d/ are automatically
# executed during container startup
COPY docker/mysql/initdb.sql /docker-entrypoint-initdb.d/
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
COPY docker/mysql/migration.sh /docker-entrypoint-initdb.d/

# Configure golang environment to run migration against database
RUN apt-get update && apt-get install -y curl git binutils bison gcc make golang

RUN mkdir /home/mysql && chown mysql:mysql /home/mysql

USER mysql

RUN bash -c "bash < <(curl -s -S -L https://raw.githubusercontent.com/moovweb/gvm/master/binscripts/gvm-installer)"
RUN bash -c "source /home/mysql/.gvm/scripts/gvm && gvm install go1.13"

COPY --chown=mysql:mysql . /home/mysql/contest

WORKDIR /home/mysql

RUN bash -c "source .gvm/scripts/gvm && \
             cd contest && \
             gvm use 1.13 && gvm linkthis github.com/facebookincubator/contest"

USER root
