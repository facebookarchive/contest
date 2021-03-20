package logrusctx

import (
	"runtime"
	"testing"
	"time"

	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/require"
)

func TestCompactTextFormatterFormat(t *testing.T) {
	formatter := &CompactTextFormatter{
		TimestampFormat: "05.999999999",
	}

	b, err := formatter.Format(&logrus.Entry{
		Data: map[string]interface{}{
			"key0": "value0",
			"key1": "value1",
		},
		Time: time.Unix(1, 2),
		Caller: &runtime.Frame{
			Function: "func",
			File:     "/directory/file",
			Line:     3,
		},
		Message: "message",
	})
	require.NoError(t, err)
	require.Equal(t, "[01.000000002 U file:3] message\tkey0=value0\tkey1=value1\n", string(b))
}
