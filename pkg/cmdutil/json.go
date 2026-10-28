package cmdutil

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

// JSONFlags holds --json, --jq and --template flag state.
type JSONFlags struct {
	Fields   []string
	JQ       string
	Template string

	// ctx is the command's context, saved before the command runs, so a
	// --jq filter stops when the command is canceled.
	ctx context.Context
}

// AddJSONFlags adds --json, --jq and --template to a command. --json
// accepts only validFields, which are the JSON keys of the command's
// output; --jq and --template require --json, exclude each other and are
// checked before the command runs.
func AddJSONFlags(cmd *cobra.Command, jf *JSONFlags, validFields []string) {
	cmd.Flags().Var(&jsonFieldsValue{fields: &jf.Fields, valid: validFields}, "json",
		fmt.Sprintf("Output JSON with the specified `fields`: %s", strings.Join(validFields, ", ")))
	cmd.Flags().StringVarP(&jf.JQ, "jq", "q", "", "Filter JSON output using a jq `expression`")
	cmd.Flags().StringVarP(&jf.Template, "template", "t", "",
		"Format JSON output using a Go `template` (helpers: join, pluck, truncate, timeago, tablerow, tablerender)")

	preRunE := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		jf.ctx = c.Context()
		if jf.JQ != "" && jf.Template != "" {
			return errors.New("cannot use `--jq` and `--template` together")
		}
		if jf.JQ != "" {
			if !jf.IsJSON() {
				return errors.New("cannot use `--jq` without specifying `--json`")
			}
			if _, err := compileJQ(jf.JQ); err != nil {
				return err
			}
		}
		if jf.Template != "" {
			if !jf.IsJSON() {
				return errors.New("cannot use `--template` without specifying `--json`")
			}
			if _, err := parseTemplate(jf.Template); err != nil {
				return err
			}
		}
		if preRunE != nil {
			return preRunE(c, args)
		}
		return nil
	}
}

// IsJSON returns true if --json was specified.
func (jf *JSONFlags) IsJSON() bool {
	return jf.Fields != nil
}

// Print writes v as JSON with only the --json fields: an array of objects
// for a slice, one object otherwise. With --jq the result goes through
// FilterJQ, with --template through ExecuteTemplate, instead of being
// printed as is.
func (jf *JSONFlags) Print(w io.Writer, v any) error {
	data, err := selectFields(v, jf.Fields)
	if err != nil {
		return err
	}
	if jf.JQ != "" {
		ctx := jf.ctx
		if ctx == nil {
			ctx = context.Background()
		}
		return FilterJQ(ctx, w, data, jf.JQ)
	}
	if jf.Template != "" {
		return ExecuteTemplate(w, data, jf.Template)
	}
	return PrintJSON(w, data)
}

// PrintJSON writes v as indented JSON to w.
func PrintJSON(w io.Writer, v interface{}) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

// FilterJQ runs the jq expression on input, a value decoded from JSON, and
// writes each result on its own line, as gh does: a string raw, null as an
// empty line, anything else as compact JSON. It stops with ctx's error when
// ctx is canceled.
func FilterJQ(ctx context.Context, w io.Writer, input any, expr string) error {
	code, err := compileJQ(expr)
	if err != nil {
		return err
	}
	iter := code.RunWithContext(ctx, input)
	for {
		v, ok := iter.Next()
		if !ok {
			return nil
		}
		if err, ok := v.(error); ok {
			return err
		}
		var line string
		switch tv := v.(type) {
		case string:
			line = tv
		case nil:
			line = ""
		default:
			b, _ := gojq.Marshal(tv) // gojq's encoder never returns an error
			line = string(b)
		}
		if _, err := fmt.Fprintln(w, line); err != nil {
			return err
		}
	}
}

// compileJQ parses and compiles a --jq expression with gojq's defaults: no
// environment loader, input iterator or module loader, so $ENV and env are
// empty and input, inputs, import and include fail
// (TestFilterJQ_NoOutsideAccess). Unlike gh, do not add them.
func compileJQ(expr string) (*gojq.Code, error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq expression: %w", err)
	}
	code, err := gojq.Compile(query)
	if err != nil {
		return nil, fmt.Errorf("invalid --jq expression: %w", err)
	}
	return code, nil
}

// selectFields round-trips v through JSON and keeps only the given keys of
// each object. Numbers stay json.Number, so large IDs keep every digit.
func selectFields(v any, fields []string) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var data any
	if err := dec.Decode(&data); err != nil {
		return nil, err
	}
	switch tv := data.(type) {
	case []any:
		for i, item := range tv {
			tv[i] = pick(item, fields)
		}
		return tv, nil
	default:
		return pick(tv, fields), nil
	}
}

func pick(v any, fields []string) any {
	obj, ok := v.(map[string]any)
	if !ok {
		return v
	}
	out := make(map[string]any, len(fields))
	for _, f := range fields {
		if val, ok := obj[f]; ok {
			out[f] = val
		}
	}
	return out
}

// jsonFieldsValue is the --json flag: a comma-separated field list that
// rejects a field outside the valid list while the flags are parsed.
type jsonFieldsValue struct {
	fields *[]string
	valid  []string
}

func (v *jsonFieldsValue) String() string {
	return strings.Join(*v.fields, ",")
}

func (v *jsonFieldsValue) Type() string { return "fields" }

func (v *jsonFieldsValue) Set(s string) error {
	for _, f := range strings.Split(s, ",") {
		f = strings.TrimSpace(f)
		if !slices.Contains(v.valid, f) {
			return fmt.Errorf("unknown JSON field: %q\nAvailable fields:\n  %s", f, strings.Join(v.valid, "\n  "))
		}
		if !slices.Contains(*v.fields, f) {
			*v.fields = append(*v.fields, f)
		}
	}
	return nil
}
