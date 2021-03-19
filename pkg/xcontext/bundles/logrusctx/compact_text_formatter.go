package logrusctx

import (
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/facebookincubator/contest/pkg/xcontext/logger"
	"github.com/sirupsen/logrus"
)

var logLevelSymbol [logger.EndOfLevel]byte

func init() {
	for level := logger.Level(0); level < logger.EndOfLevel; level++ {
		logLevelSymbol[level] = strings.ToUpper(level.String()[:1])[0]
	}
}

type CompactTextFormatter struct {
	TimestampFormat string
}

func (f *CompactTextFormatter) Format(entry *logrus.Entry) ([]byte, error) {
	var str, header strings.Builder
	timestamp := time.RFC3339
	if f.TimestampFormat != "" {
		timestamp = f.TimestampFormat
	}
	header.WriteString(fmt.Sprintf("%s %c",
		entry.Time.Format(timestamp),
		logLevelSymbol[entry.Level],
	))
	if entry.Caller != nil {
		header.WriteString(fmt.Sprintf(" %s:%d", filepath.Base(entry.Caller.File), entry.Caller.Line))
	}
	str.WriteString(fmt.Sprintf("[%s] %s",
		header.String(),
		entry.Message,
	))

	keys := make([]string, 0, len(entry.Data))
	for key := range entry.Data {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		str.WriteString(fmt.Sprintf("\t%s=%s", key, entry.Data[key]))
	}

	str.WriteByte('\n')
	return []byte(str.String()), nil
}
