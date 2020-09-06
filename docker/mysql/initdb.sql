-- Copyright (c) Facebook, Inc. and its affiliates.
--
-- This source code is licensed under the MIT license found in the
-- LICENSE file in the root directory of this source tree.

CREATE DATABASE IF NOT EXISTS contest;
CREATE DATABASE IF NOT EXISTS contest_integ;

/* 
 * use mysql_native_password as auth method because caching_sha2_password is
 * not supported by mariadb and it's the default for MySQL (starting from 8.0).
 * See https://mariadb.com/kb/en/library/authentication-plugin-sha-256/
 */
CREATE USER IF NOT EXISTS 'contest'@'%' IDENTIFIED BY 'contest';
GRANT ALL ON contest.* TO 'contest'@'%';
GRANT ALL ON contest_integ.* TO 'contest'@'%';
FLUSH PRIVILEGES;

USE contest;
source create_contest_db.sql

USE contest_integ;
source create_contest_db.sql
