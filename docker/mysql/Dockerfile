FROM mysql
# All scripts in docker-entrypoint-initdb.d/ are automatically
# executed during container startup
COPY docker/mysql/initdb.sql /docker-entrypoint-initdb.d/
COPY docker/mysql/create_contest_db.sql /
