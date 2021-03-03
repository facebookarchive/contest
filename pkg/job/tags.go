// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package job

import (
	"fmt"
	"regexp"
	"strings"
)

// InternalTagPrefix denotes tags that can only be manipulated by the server.
const InternalTagPrefix = "_"

var validTagRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

// IsValidTag validates the format of tags, only allowing [a-zA-Z0-9_-].
func IsValidTag(tag string, allowInternal bool) error {
	if !validTagRegex.MatchString(tag) {
		return fmt.Errorf("%q is not a valid tag", tag)
	}
	if !allowInternal && IsInternalTag(tag) {
		return fmt.Errorf("%q is an internal tag and is not allowed", tag)
	}
	return nil
}

// IsInternalTag returns true if a tag is an internal tag.
func IsInternalTag(tag string) bool {
	return strings.HasPrefix(tag, InternalTagPrefix)
}

// CheckTags validates the format of tags, only allowing [a-zA-Z0-9_-] and checking for dups.
func CheckTags(tags []string, allowInternal bool) error {
	tm := make(map[string]bool, len(tags))
	for _, tag := range tags {
		if err := IsValidTag(tag, allowInternal); err != nil {
			return err
		}
		if tm[tag] {
			return fmt.Errorf("duplicate tag %q", tag)
		}
		tm[tag] = true
	}
	return nil
}

// AddTags adds one ore more tags to tags unless already present.
func AddTags(tags []string, moreTags ...string) []string {
loop:
	for _, newTag := range moreTags {
		for _, tag := range tags {
			if tag == newTag {
				continue loop
			}
		}
		tags = append(tags, newTag)
	}
	return tags
}
