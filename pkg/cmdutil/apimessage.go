package cmdutil

import (
	"encoding/json"
	"io"
	"unicode/utf8"
)

const (
	// MaxErrorBody is how much of an error response ReadAPIMessage reads.
	MaxErrorBody = 64 << 10
	// MaxAPIMessage is the longest API message ReadAPIMessage returns, in
	// bytes.
	MaxAPIMessage = 512
)

// ReadAPIMessage returns the "message" of the JSON error body r, read up to
// MaxErrorBody bytes and cut to MaxAPIMessage bytes on a rune boundary, or
// "" when the body has none or is not JSON within that limit. A server can
// then not make copia-cli read or print an error of any size.
func ReadAPIMessage(r io.Reader) string {
	data, err := io.ReadAll(io.LimitReader(r, MaxErrorBody))
	if err != nil {
		return ""
	}
	var apiErr struct {
		Message string `json:"message"`
	}
	if json.Unmarshal(data, &apiErr) != nil {
		return ""
	}
	msg := apiErr.Message
	if len(msg) > MaxAPIMessage {
		msg = msg[:MaxAPIMessage]
		for !utf8.ValidString(msg) {
			msg = msg[:len(msg)-1]
		}
	}
	return msg
}
