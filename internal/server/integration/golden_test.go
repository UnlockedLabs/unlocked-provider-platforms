package integration

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// timeKey matches the JSON fields that hold a time that changes every run.
var timeKey = regexp.MustCompile(`(_at|_ts)$`)

// expectGolden compares body with testdata/<dir>/<name>.golden. A JSON body is
// compared after indenting it and replacing its times with "<time>". The
// files are copied from core and are never regenerated here.
func expectGolden(t *testing.T, dir, name, body string) {
	t.Helper()
	got := normalizeBody(t, body)
	path := filepath.Join("testdata", dir, name+".golden")
	want, err := os.ReadFile(path)
	require.NoError(t, err, "missing golden file")
	require.Equal(t, string(want), got)
}

func normalizeBody(t *testing.T, body string) string {
	t.Helper()
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return body
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	require.NoError(t, enc.Encode(maskTimes(v)))
	return buf.String()
}

// maskTimes replaces every time with "<time>", except a zero time, which is
// kept because it shows a field the route never set.
func maskTimes(v any) any {
	switch v := v.(type) {
	case map[string]any:
		for k, x := range v {
			if s, ok := x.(string); ok && timeKey.MatchString(k) && !strings.HasPrefix(s, "0001-01-01") {
				v[k] = "<time>"
			} else {
				v[k] = maskTimes(x)
			}
		}
	case []any:
		for i, x := range v {
			v[i] = maskTimes(x)
		}
	}
	return v
}

// maskFields replaces the named JSON fields, wherever they are, with
// "<masked>" unless they're empty. A body that isn't JSON is returned as is.
func maskFields(t *testing.T, body string, keys ...string) string {
	t.Helper()
	if len(keys) == 0 {
		return body
	}
	var v any
	if err := json.Unmarshal([]byte(body), &v); err != nil {
		return body
	}
	var mask func(any)
	mask = func(v any) {
		switch v := v.(type) {
		case map[string]any:
			for k, x := range v {
				if s, ok := x.(string); ok && s != "" && slices.Contains(keys, k) {
					v[k] = "<masked>"
				} else {
					mask(x)
				}
			}
		case []any:
			for _, x := range v {
				mask(x)
			}
		}
	}
	mask(v)
	out, err := json.Marshal(v)
	require.NoError(t, err)
	return string(out)
}
