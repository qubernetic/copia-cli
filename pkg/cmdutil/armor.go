package cmdutil

import "strings"

// ArmorKind is what CheckArmor finds in the armor headers of a key file.
type ArmorKind int

const (
	// ArmorPublic: at least one armor header, and every one of them is the
	// public key header.
	ArmorPublic ArmorKind = iota
	// ArmorPrivate: an armor header names a private or secret key.
	ArmorPrivate
	// ArmorOther: no armor header, or one that is neither.
	ArmorOther
)

// CheckArmor reads every line of content that holds "-----BEGIN ", also
// past its start. It is an allowlist: content is ArmorPublic only when it
// has such a line and each is exactly publicHeader (surrounding whitespace
// aside), so no other block, a private key of any format included, goes to
// the server with a public key. A header that names PRIVATE or SECRET, in
// any case and anywhere in the content, makes it ArmorPrivate.
func CheckArmor(content, publicHeader string) ArmorKind {
	public, other := false, false
	for _, line := range strings.Split(content, "\n") {
		if !strings.Contains(line, "-----BEGIN ") {
			continue
		}
		upper := strings.ToUpper(line)
		if strings.Contains(upper, "PRIVATE") || strings.Contains(upper, "SECRET") {
			return ArmorPrivate
		}
		if strings.TrimSpace(line) == publicHeader {
			public = true
		} else {
			other = true
		}
	}
	if public && !other {
		return ArmorPublic
	}
	return ArmorOther
}
