-- Copyright (c) Facebook, Inc. and its affiliates.
--
-- This source code is licensed under the MIT license found in the
-- LICENSE file in the root directory of this source tree.

-- +goose Up

ALTER TABLE test_events MODIFY COLUMN payload MEDIUMTEXT;
ALTER TABLE framework_events MODIFY COLUMN payload MEDIUMTEXT;

-- +goose Down

ALTER TABLE test_events MODIFY COLUMN payload TEXT;
ALTER TABLE framework_events MODIFY COLUMN payload TEXT;
