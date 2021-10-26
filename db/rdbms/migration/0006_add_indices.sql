-- Copyright (c) Facebook, Inc. and its affiliates.
--
-- This source code is licensed under the MIT license found in the
-- LICENSE file in the root directory of this source tree.

-- +goose Up

ALTER TABLE test_events ADD INDEX fetch_index_0 (job_id, run_id, test_name);
ALTER TABLE framework_events ADD INDEX fetch_index_0 (job_id, event_name);
ALTER TABLE run_reports ADD INDEX job_id_idx (job_id);
ALTER TABLE final_reports ADD INDEX job_id_idx (job_id);

-- +goose Down

ALTER TABLE test_events DROP INDEX fetch_index_0;
ALTER TABLE framework_events DROP INDEX fetch_index_0;
ALTER TABLE run_reports DROP INDEX job_id_idx;
ALTER TABLE final_reports DROP INDEX job_id_idx;
