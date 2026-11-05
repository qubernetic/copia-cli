package cmdutil

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/qubernetic/copia-cli/pkg/iostreams"
)

// ErrEmptyEdit is returned when the text saved in the editor is blank.
var ErrEmptyEdit = errors.New("aborted: the editor text is empty")

// Editor returns the editor command of --editor, resolving in order:
// COPIA_EDITOR, the config key editor, VISUAL, then EDITOR.
func (f *Factory) Editor() string {
	if value := os.Getenv("COPIA_EDITOR"); value != "" {
		return value
	}
	if value := f.ConfigValue("editor"); value != "" {
		return value
	}
	if value := os.Getenv("VISUAL"); value != "" {
		return value
	}
	return os.Getenv("EDITOR")
}

// EditText writes initial to a new temporary file, runs the editor command
// on it with the terminal of ios and returns the saved text. The command is
// split into words with SplitCommand and run without a shell, the file name
// appended as the last argument. The file is removed afterwards.
func EditText(ios *iostreams.IOStreams, command, initial string) (string, error) {
	words, err := SplitCommand(command)
	if err != nil {
		return "", fmt.Errorf("invalid editor command %q: %w", command, err)
	}
	if len(words) == 0 {
		return "", errors.New("no editor set: set COPIA_EDITOR, the config key editor (copia-cli config set editor <command>), VISUAL or EDITOR")
	}

	f, err := os.CreateTemp("", "copia-*.md")
	if err != nil {
		return "", err
	}
	path := f.Name()
	defer func() { _ = os.Remove(path) }()
	_, err = f.WriteString(initial)
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return "", err
	}

	cmd := exec.Command(words[0], append(words[1:], path)...)
	cmd.Stdin = ios.In
	cmd.Stdout = ios.Out
	cmd.Stderr = ios.ErrOut
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("running the editor %q: %w", words[0], err)
	}

	text, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(text), nil
}

// EditTitleBody opens the editor on the title, a blank line and the body,
// and returns the first non-blank line of the saved text as the title and
// the rest as the body. Blank text returns ErrEmptyEdit.
func EditTitleBody(ios *iostreams.IOStreams, command, title, body string) (string, string, error) {
	initial := ""
	if title != "" || body != "" {
		initial = title + "\n\n" + body
	}
	text, err := EditText(ios, command, initial)
	if err != nil {
		return "", "", err
	}
	text = strings.TrimLeft(normalizeNewlines(text), " \t\n")
	if text == "" {
		return "", "", ErrEmptyEdit
	}
	title, body, _ = strings.Cut(text, "\n")
	return strings.TrimSpace(title), trimBlankLines(body), nil
}

// EditBody opens the editor on an empty file and returns the saved text
// without leading blank lines and trailing whitespace. Blank text returns
// ErrEmptyEdit.
func EditBody(ios *iostreams.IOStreams, command string) (string, error) {
	text, err := EditText(ios, command, "")
	if err != nil {
		return "", err
	}
	body := trimBlankLines(normalizeNewlines(text))
	if strings.TrimSpace(body) == "" {
		return "", ErrEmptyEdit
	}
	return body, nil
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

// trimBlankLines removes leading blank lines and trailing whitespace, and
// keeps the indentation of the first line.
func trimBlankLines(s string) string {
	s = strings.TrimRight(s, " \t\n")
	for {
		line, rest, found := strings.Cut(s, "\n")
		if !found || strings.TrimSpace(line) != "" {
			return s
		}
		s = rest
	}
}

// SplitCommand splits a command into words the way a POSIX shell does,
// without expanding anything: words are separated by blanks, single quotes
// keep everything literally, double quotes keep everything but a backslash
// before \, ", $ or `, and a backslash outside quotes escapes the next
// character. gh splits its editor command the same way.
func SplitCommand(s string) ([]string, error) {
	var words []string
	var word strings.Builder
	inWord := false
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch c {
		case ' ', '\t', '\n':
			if inWord {
				words = append(words, word.String())
				word.Reset()
				inWord = false
			}
			continue
		case '\'':
			end := strings.IndexByte(s[i+1:], '\'')
			if end < 0 {
				return nil, errors.New("unterminated single quote")
			}
			word.WriteString(s[i+1 : i+1+end])
			i += end + 1
		case '"':
			i++
			for ; i < len(s) && s[i] != '"'; i++ {
				if s[i] == '\\' && i+1 < len(s) && strings.IndexByte("\\\"$`", s[i+1]) >= 0 {
					i++
				}
				word.WriteByte(s[i])
			}
			if i >= len(s) {
				return nil, errors.New("unterminated double quote")
			}
		case '\\':
			if i+1 >= len(s) {
				return nil, errors.New("unterminated escape")
			}
			i++
			word.WriteByte(s[i])
		default:
			word.WriteByte(c)
		}
		inWord = true
	}
	if inWord {
		words = append(words, word.String())
	}
	return words, nil
}
