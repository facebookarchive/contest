// Copyright (c) Facebook, Inc. and its affiliates.
//
// This source code is licensed under the MIT license found in the
// LICENSE file in the root directory of this source tree.

package logging

import (
	"io/ioutil"

	log_prefixed "github.com/chappjc/logrus-prefix"
	"github.com/sirupsen/logrus"
)

var (
	log *logrus.Logger
)

// GetLogger returns a configured logger instance
func GetLogger(prefix string) *logrus.Entry {
	return log.WithField("prefix", prefix)
}

// Disable sends all logging output to the bit bucket.
func Disable() {
	log.SetOutput(ioutil.Discard)
}

func init() {
	log = logrus.New()
	log.SetFormatter(&log_prefixed.TextFormatter{
		FullTimestamp: true,
	})
}
