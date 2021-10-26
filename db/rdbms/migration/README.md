# Migrations
Directory [db/rdbms/migration]( ) contains all migrations which are supported by ConTest. These consist both in `.sql` and pure go migrations as explained in the documentation [for the migration tool](https://github.com/facebookincubator/contest/tree/master/tools/migration/rdbms). Please refer to the documentation of the tool to understand how the migrations are applied. This document explains each supported migration.

Every migration is associated to a version number, which eventually will represent the db schema version.


# 0001_add_extended_descriptor.sql

The [add_extended_descriptor](https://github.com/facebookincubator/contest/blob/master/db/rdbms/migration/0001_add_extended_descriptor.sql
) migrations adds `extended_descriptor` column. This was part of [#118](https://github.com/facebookincubator/contest/pull/118), which fixed an issue in handling job descriptor and test steps descriptors. `extended_descriptor` field is introduced in the jobs table to track the initial job descriptor submitted by the user, and the resolved test descriptors (which are obtained by the test fetcher at runtime, just after submitting the job) in a single data structure. Before, they would be part of the `Request` object, populated at different times of the lifecycle of the job. This also resulted in an additional bug when rebuilding status of a job: we would fetch test descriptors from backend, instead of using the object serialized at event submission time.


# 0002_migrate_descriptor_to_extended_descriptor.go

The [migrate_descriptor_to_extended_descrptor](https://github.com/facebookincubator/contest/blob/master/db/rdbms/migration/0002_migrate_descriptor_to_extended_descriptor.go) migration backfills existing entries in the `jobs` table and populates them with an `extended_descriptor`.  After [#118](https://github.com/facebookincubator/contest/pull/118), ConTest has a dependency over the `extended_descriptor` being populated and it won't be able to re-build status for any job which hasn't had the `extended_descriptor` backfilled.

# 0003_add_jobs_state_column.sql

The [add_jobs_state_column](https://github.com/facebookincubator/contest/blob/master/db/rdbms/migration/0003_add_jobs_state_column.sql) migration adds the `state` column to the jobs table to keep track of the job state without making queries against the framework_events table. Migration script backfills the column for existing jobs and server maintains the column going forward.

# 0004_add_job_tags_table.sql

The [add_job_tags_table](https://github.com/facebookincubator/contest/blob/master/db/rdbms/migration/0004_add_job_tags_table.sql) migration creates the `job_tags` table that maintains the relationship between job id and job tags to facilitate efficient lookups of jobs by tag(s). Migration script backfills the column for existing jobs and server maintains the column going forward.

# 0005_event_payload_mediumtext.sql

`TBD`

# 0006_add_indices.sql

The [add_indices](0006_add_indices.sql) migration creates the indices required to cover SELECT requests issued by `JobRunner`.
