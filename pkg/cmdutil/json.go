package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strings"

	"github.com/itchyny/gojq"
	"github.com/spf13/cobra"
)

// JSONFlags holds --json and --jq flag state.
type JSONFlags struct {
	Fields []string
	JQ     string
}

// AddJSONFlags adds --json and --jq to a command. --json accepts only
// validFields, which are the JSON keys of the command's output; --jq
// requires --json and is checked before the command runs.
func AddJSONFlags(cmd *cobra.Command, jf *JSONFlags, validFields []string) {
	cmd.Flags().Var(&jsonFieldsValue{fields: &jf.Fields, valid: validFields}, "json",
		fmt.Sprintf("Output JSON with the specified `fields`: %s", strings.Join(validFields, ", ")))
	cmd.Flags().StringVarP(&jf.JQ, "jq", "q", "", "Filter JSON output using a jq `expression`")

	preRunE := cmd.PreRunE
	cmd.PreRunE = func(c *cobra.Command, args []string) error {
		if jf.JQ != "" {
			if !jf.IsJSON() {
				return errors.New("cannot use `--jq` without specifying `--json`")
			}
			if _, err := compileJQ(jf.JQ); err != nil {
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
// FilterJQ instead of being printed as is.
func (jf *JSONFlags) Print(w io.Writer, v any) error {
	data, err := selectFields(v, jf.Fields)
	if err != nil {
		return err
	}
	if jf.JQ != "" {
		return FilterJQ(w, data, jf.JQ)
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
// empty line, anything else as compact JSON.
func FilterJQ(w io.Writer, input any, expr string) error {
	code, err := compileJQ(expr)
	if err != nil {
		return err
	}
	iter := code.Run(input)
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
