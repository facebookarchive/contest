-- Copyright (c) Facebook, Inc. and its affiliates.
--
-- This source code is licensed under the MIT license found in the
-- LICENSE file in the root directory of this source tree.

-- +goose Up

CREATE TABLE job_tags (
  job_id BIGINT NOT NULL,
  tag VARCHAR(32),
  PRIMARY KEY (job_id, tag),
  KEY(tag)
);

-- Extract tags from the descriptor field.
SET SQL_BIG_SELECTS=1;
INSERT INTO
  job_tags
SELECT
  job_id,
  job_tags.tag AS tag
FROM
  jobs,
  JSON_TABLE(jobs.descriptor, '$.Tags[*]' COLUMNS (tag TEXT PATH '$[0]')) AS job_tags;

-- +goose Down

DROP TABLE job_tags;
