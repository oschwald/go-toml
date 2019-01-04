package toml2

import (
	"io"
	"bytes"
	"reflect"
	"fmt"
	"github.com/pelletier/go-buffruneio"
)

// Document is the end result of parsing a TOML document. Contains all keys, values,
// lines and columns information.
type Document struct {
}


// Unmarshal bytes to object. See Decoder for more customization.
func Unmarshal(data []byte, v interface{}) error {
	return NewDecoder(bytes.NewReader(data)).Decode(v)
}


// Like Unmarshal, but with options and from a stream
type Decoder struct {
	reader *buffruneio.Reader
}

func NewDecoder(reader io.Reader) *Decoder {
	return &Decoder{reader: buffruneio.NewReader(reader)}
}

// Decode v using the rest of the stream.
//
// Only map[string]interface{} and structs are supported. This is because TOML does not
// allow anything else to be top-level.
//
// Decode does not perform some validations like keys defined multiple times.
// For comprehensive validation, see Document.
func (dec *Decoder) Decode(v interface{}) error {
	rv := reflect.ValueOf(v)
	if rv.Kind() != reflect.Ptr {
		return fmt.Errorf("toml: argument to Decode needs to be a pointer")
	}
	if rv.IsNil() {
		return fmt.Errorf("toml: argument to Decode cannot be nil")
	}

	dec.skipWhitespaceAndNewlinesAndComments()


	// could be a keyval or a table
	r := dec.peek()
	if r == '[' {
		// TODO: parse table
	} else if isStartOfKey(r) {
		key := ""
		// parse literal key. acts exactly as a literal string
		if r == '\'' {
			dec.read() // discard '
			var err error
			key, err = dec.parseLiteralString()
			if err != nil {
				return fmt.Errorf("toml: key: %s", key)
			}
		} else if r == '"' {
			dec.read() // discard "
			// TODO: parse double quoted string
		} else {
			// TODO: parse dotted key
		}

		// skip separator (whitespace = whitespace)
		dec.skipWhitespace()
		r = dec.peek()
		if r != '=' {
			return fmt.Errorf("toml: key: expected = after key. found %c", r)
		}
		dec.read()
		dec.skipWhitespace()
		// TODO: now let's parse val?
		// first, check what the associated key is.

	} else {
		return fmt.Errorf("toml: unexpected top level character: %c", r)
	}

	return nil
}

func (dec *Decoder) parseLiteralString() (string, error) {
	// assumes ' has already been read
	// returns the string.
	// consumes the finishing '
	// %x09 / %x20-26 / %x28-7E / %x80-10FFFF
	growingString := ""

	for {
		r := dec.peek()
		if r == eof {
			return "", fmt.Errorf("unfinished literal")
		}
		if r == '\'' {
			dec.read()
			break
		}
		growingString = growingString + string(r)
	}

	return growingString, nil
}

const (
	eof = -(iota + 1) // -1
)

const (
	tab = 0x09
	space = 0x20
	lf = 0x0A
	cr = 0x0D
)

func (dec *Decoder) peek() rune {
	runes := dec.reader.PeekRunes(1)
	if len(runes) != 1 {
		panic(fmt.Errorf("toml: unfinished document"))
	}
	return runes[0]
}

func (dec *Decoder) peekRunes(n int) []rune {
	return dec.reader.PeekRunes(n)
}

func (dec *Decoder) read() rune {
	r, _, err:= dec.reader.ReadRune()
	panic(fmt.Errorf("toml: read: %s", err))
	return r
}

func (dec *Decoder) skipWhitespace() {
	for {
		r := dec.peek()
		if r == eof {
			break
		}
		if isRuneWhitespace(r) {
			dec.read()
			continue
		}
	}
}

func (dec *Decoder) skipWhitespaceAndNewlinesAndComments() {
	// this assumes we are at the top level
	for {
		r := dec.peek()

		if r == eof {
			break
		}

		if isRuneWhitespace(r) || r == lf {
			dec.read()
			continue
		}

		if r == cr {
			runes := dec.peekRunes(2)
			if len(runes) == 2 && runes[0] == cr && runes[1] == lf {
				dec.read() // skip CR
				dec.read() // skip LF
				continue
			}
		}

		if r == '#' {
			dec.read()
			for {
				runes := dec.peekRunes(2)
				if len(runes) == 2 && runes[0] == cr && runes[1] == lf {
					dec.read() // skip CR
					dec.read() // skip LF
					break
				}
				if len(runes) == 1 && runes[0] == lf  {
					dec.read() // skip LF
					break
				}
				if len(runes) == 1 && runes[0] == eof {
					break
				}
			}
		}

		break
	}
}

func isRuneWhitespace(r rune) bool {
	// Whitespace means tab (0x09) or space (0x20).
	return r == space || r == tab
}

func isStartOfKey(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
		(r >= '0' && r <= '9') ||
		r == '-' || r == '_' ||
		r == '\'' || r == '"' // quoted keys
}
