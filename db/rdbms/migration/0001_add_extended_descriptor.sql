-- Copyright (c) Facebook, Inc. and its affiliates.
--
-- This source code is licensed under the MIT license found in the
-- LICENSE file in the root directory of this source tree.
-- +goose Up
ALTER TABLE jobs ADD COLUMN extended_descriptor text;

-- +goose Down
-- Down migration is not supported and it's a no-op
