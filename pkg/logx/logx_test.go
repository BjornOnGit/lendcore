package logx

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// decode parses each non-empty line of buf as a JSON object.
func decode(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	trimmed := strings.TrimSpace(buf.String())
	if trimmed == "" {
		return nil
	}
	var out []map[string]any
	for _, line := range strings.Split(trimmed, "\n") {
		var m map[string]any
		require.NoError(t, json.Unmarshal([]byte(line), &m), "line is not valid JSON: %q", line)
		out = append(out, m)
	}
	return out
}

func TestNewReturnsLogger(t *testing.T) {
	assert.NotNil(t, New("info"))
}

func TestLoggerEmitsJSON(t *testing.T) {
	var buf bytes.Buffer
	newLogger(&buf, "info").Info("hello", "k", "v")

	lines := decode(t, &buf)
	require.Len(t, lines, 1)
	assert.Equal(t, "INFO", lines[0]["level"])
	assert.Equal(t, "hello", lines[0]["msg"])
	assert.Equal(t, "v", lines[0]["k"])
	assert.Contains(t, lines[0], "time")
}

func TestLevelFiltering(t *testing.T) {
	tests := []struct {
		level string
		want  []string
	}{
		{"debug", []string{"d", "i", "w", "e"}},
		{"info", []string{"i", "w", "e"}},
		{"WARN", []string{"w", "e"}},
		{"error", []string{"e"}},
	}
	for _, tt := range tests {
		t.Run(tt.level, func(t *testing.T) {
			var buf bytes.Buffer
			l := newLogger(&buf, tt.level)
			l.Debug("d")
			l.Info("i")
			l.Warn("w")
			l.Error("e")

			var got []string
			for _, line := range decode(t, &buf) {
				got = append(got, line["msg"].(string))
			}
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestUnrecognizedLevelFallsBackToInfo(t *testing.T) {
	var buf bytes.Buffer
	l := newLogger(&buf, "bogus")
	l.Debug("hidden")
	l.Info("shown")

	lines := decode(t, &buf)
	require.Len(t, lines, 2)
	assert.Equal(t, "WARN", lines[0]["level"])
	assert.Equal(t, "unrecognized log level, using info", lines[0]["msg"])
	assert.Equal(t, "bogus", lines[0]["requested_level"])
	assert.Equal(t, "shown", lines[1]["msg"])
}

func TestRequestIDRoundTrip(t *testing.T) {
	ctx := context.Background()
	assert.Equal(t, "", RequestID(ctx))

	ctx = WithRequestID(ctx, "abc")
	assert.Equal(t, "abc", RequestID(ctx))

	assert.Equal(t, "def", RequestID(WithRequestID(ctx, "def")))
	assert.Equal(t, "abc", RequestID(ctx), "parent context is unchanged")
}
