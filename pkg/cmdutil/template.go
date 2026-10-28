package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"text/tabwriter"
	"text/template"
	"time"
)

// timeNow is the clock timeago measures against; tests replace it.
var timeNow = time.Now

// ExecuteTemplate runs the Go template tpl on data, a value decoded from
// JSON, and writes the result to w. The helpers are the ones gh offers
// without color: join, pluck, truncate, timeago, tablerow and tablerender.
// Rows from tablerow that no tablerender printed are printed at the end.
func ExecuteTemplate(w io.Writer, data any, tpl string) error {
	t, err := parseTemplate(tpl)
	if err != nil {
		return err
	}
	return t.execute(w, data)
}

type templateRun struct {
	tpl   *template.Template
	out   io.Writer
	table *bytes.Buffer
}

func parseTemplate(tpl string) (*templateRun, error) {
	r := &templateRun{}
	funcs := template.FuncMap{
		"join":        templateJoin,
		"pluck":       templatePluck,
		"truncate":    templateTruncate,
		"timeago":     templateTimeago,
		"tablerow":    r.tableRow,
		"tablerender": r.tableRender,
	}
	t, err := template.New("").Option("missingkey=zero").Funcs(funcs).Parse(tpl)
	if err != nil {
		return nil, fmt.Errorf("invalid --template: %w", err)
	}
	r.tpl = t
	return r, nil
}

func (r *templateRun) execute(w io.Writer, data any) error {
	r.out = w
	if err := r.tpl.Execute(w, data); err != nil {
		return err
	}
	_, err := r.tableRender()
	return err
}

// tableRow collects one row of the table that tableRender prints with its
// columns aligned.
func (r *templateRun) tableRow(fields ...any) (string, error) {
	if r.table == nil {
		r.table = &bytes.Buffer{}
	}
	cells := make([]string, len(fields))
	for i, f := range fields {
		s, err := jsonScalarToString(f)
		if err != nil {
			return "", fmt.Errorf("failed to write table row: %w", err)
		}
		cells[i] = s
	}
	r.table.WriteString(strings.Join(cells, "\t") + "\n")
	return "", nil
}

func (r *templateRun) tableRender() (string, error) {
	if r.table == nil {
		return "", nil
	}
	rows := r.table
	r.table = nil
	tw := tabwriter.NewWriter(r.out, 0, 0, 2, ' ', 0)
	_, err := tw.Write(rows.Bytes())
	return "", errors.Join(err, tw.Flush())
}

func templateJoin(sep string, input []any) (string, error) {
	var parts []string
	for _, item := range input {
		if item == nil {
			continue
		}
		s, err := jsonScalarToString(item)
		if err != nil {
			return "", err
		}
		parts = append(parts, s)
	}
	return strings.Join(parts, sep), nil
}

func templatePluck(field string, input []any) ([]any, error) {
	var out []any
	for _, item := range input {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("pluck: expected an object, got %T", item)
		}
		out = append(out, obj[field])
	}
	return out, nil
}

// templateTruncate cuts v to maxWidth characters, ending in "..." when
// there is room for it, as gh does.
func templateTruncate(maxWidth int, v any) (string, error) {
	if v == nil {
		return "", nil
	}
	s, ok := v.(string)
	if !ok {
		return "", fmt.Errorf("truncate: expected a string, got %T", v)
	}
	runes := []rune(s)
	if len(runes) <= maxWidth {
		return s, nil
	}
	if maxWidth >= 5 {
		return string(runes[:maxWidth-3]) + "...", nil
	}
	return string(runes[:maxWidth]), nil
}

func templateTimeago(input string) (string, error) {
	t, err := time.Parse(time.RFC3339, input)
	if err != nil {
		return "", err
	}
	ago := timeNow().Sub(t)
	switch {
	case ago < time.Minute:
		return "just now", nil
	case ago < time.Hour:
		return pluralize(int(ago.Minutes()), "minute") + " ago", nil
	case ago < 24*time.Hour:
		return pluralize(int(ago.Hours()), "hour") + " ago", nil
	case ago < 30*24*time.Hour:
		return pluralize(int(ago.Hours())/24, "day") + " ago", nil
	case ago < 365*24*time.Hour:
		return pluralize(int(ago.Hours())/24/30, "month") + " ago", nil
	default:
		return pluralize(int(ago.Hours()/24/365), "year") + " ago", nil
	}
}

func pluralize(n int, word string) string {
	if n == 1 {
		return "1 " + word
	}
	return fmt.Sprintf("%d %ss", n, word)
}

func jsonScalarToString(v any) (string, error) {
	switch tv := v.(type) {
	case string:
		return tv, nil
	case json.Number:
		return tv.String(), nil
	case float64:
		if math.Trunc(tv) == tv {
			return strconv.FormatFloat(tv, 'f', 0, 64), nil
		}
		return strconv.FormatFloat(tv, 'f', 2, 64), nil
	case nil:
		return "", nil
	case bool:
		return strconv.FormatBool(tv), nil
	default:
		return "", fmt.Errorf("cannot convert type to string: %v", tv)
	}
}
