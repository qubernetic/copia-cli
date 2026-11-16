package cmdutil

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func runTemplate(t *testing.T, tpl string, data any) (string, error) {
	t.Helper()
	var out bytes.Buffer
	err := ExecuteTemplate(&out, data, tpl)
	return out.String(), err
}

var templateItems = []any{
	map[string]any{"number": json.Number("1"), "title": "First", "draft": false, "user": map[string]any{"login": "alice"}},
	map[string]any{"number": json.Number("12"), "title": "Second", "draft": true, "user": map[string]any{"login": "bob"}},
}

func TestTemplate_Join(t *testing.T) {
	out, err := runTemplate(t, `{{join ", " .}}`, []any{"a", nil, json.Number("2"), 1.5, 3.0, true})
	require.NoError(t, err)
	assert.Equal(t, "a, 2, 1.50, 3, true", out)

	_, err = runTemplate(t, `{{join ", " .}}`, []any{map[string]any{}})
	require.ErrorContains(t, err, "cannot convert type to string: map[]")
}

func TestTemplate_Pluck(t *testing.T) {
	out, err := runTemplate(t, `{{join "," (pluck "title" .)}}`, templateItems)
	require.NoError(t, err)
	assert.Equal(t, "First,Second", out)

	_, err = runTemplate(t, `{{pluck "title" .}}`, []any{"x"})
	require.ErrorContains(t, err, "pluck: expected an object, got string")
}

func TestTemplate_Truncate(t *testing.T) {
	tests := []struct {
		tpl  string
		data any
		want string
	}{
		{tpl: `{{truncate 10 .}}`, data: "short", want: "short"},
		{tpl: `{{truncate 8 .}}`, data: "a long title", want: "a lon..."},
		{tpl: `{{truncate 4 .}}`, data: "ábcdef", want: "ábcd"},
		{tpl: `{{truncate 4 .}}`, data: nil, want: ""},
	}
	for _, tt := range tests {
		out, err := runTemplate(t, tt.tpl, tt.data)
		require.NoError(t, err)
		assert.Equal(t, tt.want, out)
	}

	_, err := runTemplate(t, `{{truncate 4 .}}`, json.Number("12345"))
	require.ErrorContains(t, err, "truncate: expected a string, got json.Number")
}

func TestTemplate_Timeago(t *testing.T) {
	now := time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)
	t.Cleanup(func() { timeNow = time.Now })
	timeNow = func() time.Time { return now }

	tests := []struct {
		ago  time.Duration
		want string
	}{
		{ago: 30 * time.Second, want: "just now"},
		{ago: time.Minute, want: "1 minute ago"},
		{ago: 5 * time.Minute, want: "5 minutes ago"},
		{ago: 2 * time.Hour, want: "2 hours ago"},
		{ago: 3 * 24 * time.Hour, want: "3 days ago"},
		{ago: 61 * 24 * time.Hour, want: "2 months ago"},
		{ago: 400 * 24 * time.Hour, want: "1 year ago"},
	}
	for _, tt := range tests {
		out, err := runTemplate(t, `{{timeago .}}`, now.Add(-tt.ago).Format(time.RFC3339))
		require.NoError(t, err)
		assert.Equal(t, tt.want, out)
	}

	_, err := runTemplate(t, `{{timeago .}}`, "yesterday")
	require.ErrorContains(t, err, `parsing time "yesterday"`)
}

func TestTemplate_Tablerow(t *testing.T) {
	out, err := runTemplate(t, `{{range .}}{{tablerow .number .title .draft}}{{end}}`, templateItems)
	require.NoError(t, err)
	assert.Equal(t, "1   First   false\n12  Second  true\n", out)

	out, err = runTemplate(t, `{{tablerow "a" .missing "c"}}`, map[string]any{})
	require.NoError(t, err)
	assert.Equal(t, "a    c\n", out)

	_, err = runTemplate(t, `{{tablerow .}}`, []any{})
	require.ErrorContains(t, err, "failed to write table row: cannot convert type to string: []")
}

func TestTemplate_Tablerender(t *testing.T) {
	tpl := `{{range .}}{{tablerow .number .user.login}}{{end}}{{tablerender}}---{{"\n"}}` +
		`{{tablerow "A" "B"}}{{tablerender}}{{tablerender}}`
	out, err := runTemplate(t, tpl, templateItems)
	require.NoError(t, err)
	assert.Equal(t, "1   alice\n12  bob\n---\nA  B\n", out)
}

func TestTemplate_WriteError(t *testing.T) {
	err := ExecuteTemplate(failWriter{}, templateItems, `{{range .}}{{tablerow .title}}{{end}}`)
	require.ErrorIs(t, err, assert.AnError)

	err = ExecuteTemplate(failWriter{}, templateItems, `{{tablerow "x" "y"}}{{tablerender}}`)
	require.ErrorIs(t, err, assert.AnError)
}

func TestTemplate_Invalid(t *testing.T) {
	_, err := runTemplate(t, `{{nohelper .}}`, nil)
	require.EqualError(t, err, `invalid --template: template: :1: function "nohelper" not defined`)
}
