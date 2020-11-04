-- Copyright (c) Facebook, Inc. and its affiliates.
--
-- This source code is licensed under the MIT license found in the
-- LICENSE file in the root directory of this source tree.

CREATE TABLE test_events (
	event_id BIGINT(20) UNSIGNED NOT NULL AUTO_INCREMENT,
	job_id BIGINT(20) NOT NULL,
	run_id BIGINT(20) NOT NULL,
	test_name VARCHAR(32) NULL,
	test_step_label VARCHAR(32) NULL,
	event_name VARCHAR(32) NULL,
	target_name VARCHAR(64) NULL,
	target_id VARCHAR(64) NULL,
	payload TEXT NULL,
	emit_time TIMESTAMP NOT NULL,
	PRIMARY KEY (event_id)
);

CREATE TABLE framework_events (
	event_id BIGINT(20) UNSIGNED NOT NULL AUTO_INCREMENT,
	job_id BIGINT(20) NOT NULL,
	event_name VARCHAR(32) NULL,
	payload TEXT NULL,
	emit_time TIMESTAMP NOT NULL,
	PRIMARY KEY (event_id)
);

CREATE TABLE run_reports (
	report_id BIGINT(20) UNSIGNED NOT NULL AUTO_INCREMENT,
	job_id BIGINT(20) NOT NULL,
	run_id BIGINT(20) NOT NULL,
	reporter_name VARCHAR(32) NOT NULL,
	success TINYINT(1) NULL,
	report_time TIMESTAMP NOT NULL,
	data TEXT NOT NULL,
	PRIMARY KEY (report_id)
);

CREATE TABLE final_reports (
	report_id BIGINT(20) UNSIGNED NOT NULL AUTO_INCREMENT,
	job_id BIGINT(20) NOT NULL,
	success TINYINT(1) NULL,
	reporter_name VARCHAR(32) NOT NULL,
	report_time TIMESTAMP NOT NULL,
	data TEXT NOT NULL,
	PRIMARY KEY (report_id)
);

CREATE TABLE jobs (
	job_id BIGINT(20) UNSIGNED NOT NULL AUTO_INCREMENT,
	name VARCHAR(64) NOT NULL,
	requestor VARCHAR(32) NOT NULL,
	server_id VARCHAR(64) NOT NULL,
	request_time TIMESTAMP NOT NULL,
	descriptor TEXT NOT NULL,
	teststeps TEXT,
	PRIMARY KEY (job_id)
);

CREATE TABLE locks (
	target_id VARCHAR(64) NOT NULL,
	job_id BIGINT(20) UNSIGNED NOT NULL,
	created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	expires_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
	valid BOOL NOT NULL DEFAULT TRUE,
	PRIMARY KEY (target_id)
);
