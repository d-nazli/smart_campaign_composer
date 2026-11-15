package llm

import (
	"strconv"
	"strings"
	"unicode/utf16"
	"unicode/utf8"
)

// Delta is a piece of one string field of the streamed JSON object.
type Delta struct {
	Field string
	Text  string
}

// FieldStream extracts, incrementally, the string values of the top-level keys
// of a JSON object that arrives in arbitrary fragments. It exists so the UI can
// show the SMS and email being "typed" while the model is still producing the
// JSON. It only ever feeds the live preview: the final text is parsed and
// validated from the complete output, so a bug here cannot let bad content
// through.
//
// It handles fragment boundaries inside keys, inside escape sequences
// (\n, \", 😀 ...) and inside multi-byte UTF-8 characters.
type FieldStream struct {
	want map[string]bool

	pending []byte // incomplete UTF-8 sequence carried to the next Write

	state     fsState
	key       strings.Builder
	field     string // current value's key if wanted, else ""
	escape    bool
	unicode   []byte // hex digits of a \uXXXX being read
	highSurr  rune   // pending high surrogate
	depth     int    // nesting depth of skipped non-string values
	inSkipStr bool
	skipEsc   bool
}

type fsState int

const (
	fsBeforeKey fsState = iota
	fsKey
	fsAfterKey
	fsValue
	fsString
	fsSkip
)

// NewFieldStream returns a stream that reports only the given keys.
func NewFieldStream(fields ...string) *FieldStream {
	w := map[string]bool{}
	for _, f := range fields {
		w[f] = true
	}
	return &FieldStream{want: w}
}

// Write consumes a fragment and returns the newly decoded text per field.
func (s *FieldStream) Write(fragment string) []Delta {
	data := append(s.pending, fragment...)
	s.pending = nil

	var out []Delta
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 && s.field != "" {
			out = append(out, Delta{Field: s.field, Text: buf.String()})
		}
		buf.Reset()
	}

	for len(data) > 0 {
		if !utf8.FullRune(data) {
			s.pending = append([]byte(nil), data...)
			break
		}
		r, size := utf8.DecodeRune(data)
		data = data[size:]

		switch s.state {
		case fsBeforeKey:
			if r == '"' {
				s.state = fsKey
				s.key.Reset()
				s.escape = false
			}
		case fsKey:
			switch {
			case s.escape:
				s.key.WriteRune(r)
				s.escape = false
			case r == '\\':
				s.escape = true
			case r == '"':
				s.field = ""
				if k := s.key.String(); s.want[k] {
					s.field = k
				}
				s.state = fsAfterKey
			default:
				s.key.WriteRune(r)
			}
		case fsAfterKey:
			if r == ':' {
				s.state = fsValue
			}
		case fsValue:
			switch {
			case r == ' ' || r == '\n' || r == '\t' || r == '\r':
			case r == '"':
				s.state = fsString
				s.escape, s.unicode, s.highSurr = false, nil, 0
			default:
				// number, bool, null, array or object: skip to the next key.
				s.state = fsSkip
				s.depth, s.inSkipStr, s.skipEsc = 0, false, false
				s.skip(r)
			}
		case fsString:
			switch {
			case s.unicode != nil:
				s.unicode = append(s.unicode, byte(r))
				if len(s.unicode) == 4 {
					n, err := strconv.ParseUint(string(s.unicode), 16, 32)
					s.unicode = nil
					if err == nil {
						if c := s.combine(rune(n)); c != 0 { // 0 = waiting for the low surrogate
							buf.WriteRune(c)
						}
					}
				}
			case s.escape:
				s.escape = false
				switch r {
				case 'n':
					buf.WriteByte('\n')
				case 't':
					buf.WriteByte('\t')
				case 'r':
					buf.WriteByte('\r')
				case 'b':
					buf.WriteByte('\b')
				case 'f':
					buf.WriteByte('\f')
				case 'u':
					s.unicode = []byte{}
				default: // \" \\ \/ and anything else: the character itself
					buf.WriteRune(r)
				}
			case r == '\\':
				s.escape = true
			case r == '"':
				flush()
				s.field = ""
				s.state = fsBeforeKey
			default:
				buf.WriteRune(r)
			}
		case fsSkip:
			s.skip(r)
		}
	}
	flush()
	return out
}

// combine joins UTF-16 surrogate pairs written as two \u escapes.
func (s *FieldStream) combine(r rune) rune {
	if utf16.IsSurrogate(r) {
		if s.highSurr == 0 && r >= 0xD800 && r < 0xDC00 {
			s.highSurr = r
			return 0
		}
		if s.highSurr != 0 {
			c := utf16.DecodeRune(s.highSurr, r)
			s.highSurr = 0
			if c != utf8.RuneError {
				return c
			}
		}
		return utf8.RuneError
	}
	s.highSurr = 0
	return r
}

// skip consumes a non-string value until the comma that ends it (or the
// closing brace), tracking nesting and strings.
func (s *FieldStream) skip(r rune) {
	if s.inSkipStr {
		switch {
		case s.skipEsc:
			s.skipEsc = false
		case r == '\\':
			s.skipEsc = true
		case r == '"':
			s.inSkipStr = false
		}
		return
	}
	switch r {
	case '"':
		s.inSkipStr = true
	case '[', '{':
		s.depth++
	case ']':
		s.depth--
	case '}':
		if s.depth == 0 {
			s.state = fsBeforeKey
			return
		}
		s.depth--
	case ',':
		if s.depth == 0 {
			s.state = fsBeforeKey
		}
	}
}
