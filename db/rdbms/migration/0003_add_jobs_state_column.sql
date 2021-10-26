-- Copyright (c) Facebook, Inc. and its affiliates.
--
-- This source code is licensed under the MIT license found in the
-- LICENSE file in the root directory of this source tree.

-- +goose Up

ALTER TABLE jobs ADD COLUMN state tinyint DEFAULT 0 AFTER extended_descriptor;
CREATE INDEX job_state ON jobs (state, job_id);

-- Populate the column.

SET SQL_BIG_SELECTS=1;

UPDATE
  jobs
INNER JOIN
  ( -- This query retrieves job state event by its id.
    SELECT
      jobs.job_id AS job_id,
      fe2.event_name AS state
    FROM
      jobs
    INNER JOIN
      ( -- This query finds id of the last job state event for each job.
        SELECT
          job_id,
          MAX(event_id) AS max_event
        FROM
          framework_events fe
        WHERE
          fe.event_name IN ("JobStateStarted", "JobStateCompleted", "JobStateFailed",
                            "JobStatePaused", "JobStatePauseFailed", "JobStateCancelling",
                            "JobStateCancelled", "JobStateCancellationFailed")
        GROUP BY
          job_id
      ) fe1
    ON
      fe1.job_id = jobs.job_id
    INNER JOIN
      framework_events fe2
    ON
      fe2.event_id = fe1.max_event
  ) tt
ON
  jobs.job_id = tt.job_id
SET
  jobs.state =
      -- Translate string to numeric representation.
      IF(tt.state = "JobStateStarted", 1,
      IF(tt.state = "JobStateCompleted", 2,
      IF(tt.state = "JobStateFailed", 3,
      IF(tt.state = "JobStatePaused", 4,
      IF(tt.state = "JobStatePauseFailed", 5,
      IF(tt.state = "JobStateCancelling", 6,
      IF(tt.state = "JobStateCancelled", 7,
      IF(tt.state = "JobStateCancellationFailed", 8,
      0))))))));

-- +goose Down

DROP INDEX job_state ON jobs
ALTER TABLE jobs DROP COLUMN state;
