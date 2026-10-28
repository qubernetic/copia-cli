package cmdutil

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJSONFlags_IsJSON(t *testing.T) {
	jf := JSONFlags{}
	assert.False(t, jf.IsJSON())

	jf.Fields = []string{"name", "title"}
	assert.True(t, jf.IsJSON())
}

func TestPrintJSON(t *testing.T) {
	var buf bytes.Buffer
	data := map[string]string{"name": "test", "value": "123"}

	err := PrintJSON(&buf, data)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), `"name": "test"`)
	assert.Contains(t, buf.String(), `"value": "123"`)
}

type testItem struct {
	Number int64      `json:"number"`
	Title  string     `json:"title"`
	State  string     `json:"state"`
	User   testUser   `json:"user"`
	Labels []testUser `json:"labels"`
}

type testUser struct {
	Login string `json:"login"`
}

var testItems = []testItem{
	{Number: 1, Title: "First", State: "open", User: testUser{"alice"}, Labels: []testUser{{"bug"}}},
	{Number: 9007199254740993, Title: "Second", State: "closed", User: testUser{"bob"}},
}

var testFields = []string{"number", "title", "state", "user", "labels"}

func runJSONCmd(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var jf JSONFlags
	var out bytes.Buffer
	cmd := &cobra.Command{
		Use: "test",
		RunE: func(cmd *cobra.Command, _ []string) error {
			if jf.IsJSON() {
				return jf.Print(&out, testItems)
			}
			_, err := out.WriteString("table\n")
			return err
		},
	}
	AddJSONFlags(cmd, &jf, testFields)
	cmd.SetArgs(args)
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	err := cmd.Execute()
	return out.String(), err
}

func TestJSONFlags_Print(t *testing.T) {
	tests := []struct {
		name    string
		args    []string
		want    string
		wantErr string
	}{
		{
			name: "no flags prints the table",
			want: "table\n",
		},
		{
			name: "only the requested fields",
			args: []string{"--json", "number,title"},
			want: `[
  {
    "number": 1,
    "title": "First"
  },
  {
    "number": 9007199254740993,
    "title": "Second"
  }
]
`,
		},
		{
			name: "repeated flag adds fields",
			args: []string{"--json", "state", "--json", "user"},
			want: `[
  {
    "state": "open",
    "user": {
      "login": "alice"
    }
  },
  {
    "state": "closed",
    "user": {
      "login": "bob"
    }
  }
]
`,
		},
		{
			name:    "unknown field lists the valid ones",
			args:    []string{"--json", "number,author"},
			wantErr: "invalid argument \"number,author\" for \"--json\" flag: unknown JSON field: \"author\"\nAvailable fields:\n  number\n  title\n  state\n  user\n  labels",
		},
		{
			name:    "empty field list",
			args:    []string{"--json", ""},
			wantErr: "invalid argument \"\" for \"--json\" flag: unknown JSON field: \"\"\nAvailable fields:\n  number\n  title\n  state\n  user\n  labels",
		},
		{
			name: "jq string results print raw, one per line",
			args: []string{"--json", "title", "--jq", ".[].title"},
			want: "First\nSecond\n",
		},
		{
			name: "jq other results print as JSON",
			args: []string{"--json", "number,user,labels", "--jq", ".[] | .number, .user, .labels, (.number > 1), null"},
			want: "1\n{\"login\":\"alice\"}\n[{\"login\":\"bug\"}]\nfalse\n\n" +
				"9007199254740993\n{\"login\":\"bob\"}\n\ntrue\n\n",
		},
		{
			name: "jq sees only the requested fields",
			args: []string{"--json", "number", "--jq", ".[0] | keys"},
			want: "[\"number\"]\n",
		},
		{
			name:    "jq without json",
			args:    []string{"--jq", ".[]"},
			wantErr: "cannot use `--jq` without specifying `--json`",
		},
		{
			name:    "invalid jq expression",
			args:    []string{"--json", "title", "--jq", ".[] |"},
			wantErr: "invalid --jq expression: unexpected EOF",
		},
		{
			name:    "jq runtime error",
			args:    []string{"--json", "title", "--jq", ".title"},
			wantErr: "expected an object but got: array ([{\"title\":\"First\"},{\"titl ...])",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			out, err := runJSONCmd(t, tt.args...)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				assert.Empty(t, out)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, out)
		})
	}
}

func TestJSONFlags_PrintObject(t *testing.T) {
	jf := JSONFlags{Fields: []string{"title", "user"}}
	var out bytes.Buffer
	require.NoError(t, jf.Print(&out, testItems[0]))
	assert.Equal(t, `{
  "title": "First",
  "user": {
    "login": "alice"
  }
}
`, out.String())
}

func TestJSONFlags_PrintNonObjects(t *testing.T) {
	jf := JSONFlags{Fields: []string{"title"}}
	var out bytes.Buffer
	require.NoError(t, jf.Print(&out, []string{"a"}))
	assert.Equal(t, "[\n  \"a\"\n]\n", out.String())

	err := jf.Print(&out, map[string]any{"title": make(chan int)})
	require.EqualError(t, err, "json: unsupported type: chan int")
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, assert.AnError }

func TestFilterJQ_WriteError(t *testing.T) {
	err := FilterJQ(failWriter{}, "x", ".")
	require.ErrorIs(t, err, assert.AnError)
}

func TestJSONFlags_PrintKeepsPreRunE(t *testing.T) {
	var jf JSONFlags
	called := false
	cmd := &cobra.Command{
		Use:     "test",
		PreRunE: func(*cobra.Command, []string) error { called = true; return nil },
		RunE:    func(*cobra.Command, []string) error { return nil },
	}
	AddJSONFlags(cmd, &jf, testFields)
	cmd.SetArgs([]string{"--json", "title"})
	require.NoError(t, cmd.Execute())
	assert.True(t, called)
	assert.Equal(t, "title", cmd.Flags().Lookup("json").Value.String())
	assert.Equal(t, "fields", cmd.Flags().Lookup("json").Value.Type())
}

func TestFilterJQ(t *testing.T) {
	tests := []struct {
		name    string
		input   any
		expr    string
		want    string
		wantErr string
	}{
		{name: "string raw", input: map[string]any{"a": "x\ty"}, expr: ".a", want: "x\ty\n"},
		{name: "no output", input: []any{}, expr: ".[]", want: ""},
		{name: "html kept", input: "<a>", expr: "[.]", want: "[\"<a>\"]\n"},
		{name: "parse error", input: nil, expr: "{", wantErr: "invalid --jq expression: unexpected EOF"},
		{name: "compile error", input: nil, expr: "$x", wantErr: "invalid --jq expression: variable not defined: $x"},
		{name: "infinity", input: nil, expr: "infinite", want: "1.7976931348623157e+308\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			err := FilterJQ(&out, tt.input, tt.expr)
			if tt.wantErr != "" {
				require.EqualError(t, err, tt.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, out.String())
		})
	}
}
