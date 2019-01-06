package toml2

import (
	"bytes"
	"fmt"
	"github.com/pelletier/go-buffruneio"
	"io"
	"reflect"
	"strconv"
	"unicode"
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
		key, err := dec.parseDottedKey()
		if err != nil {
			return fmt.Errorf("toml: key: %s", err)
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

func (dec *Decoder) parseDottedKey() (string, error) {
	key, err := dec.parseSimpleKey()
	if err != nil {
		return "", err
	}
	for {
		r := dec.peek()
		if r != '.' {
			break
		}
		dec.read() // read the .
		keyPart, err := dec.parseSimpleKey()
		if err != nil {
			return "", err
		}
		key += "." + keyPart
	}
	return key, nil
}

func (dec *Decoder) parseSimpleKey() (string, error) {
	r := dec.peek()
	var key string
	if r == '\'' { // parse literal key. acts exactly as a literal string
		dec.read() // discard '
		var err error
		key, err = dec.parseLiteralString()
		if err != nil {
			return "", fmt.Errorf("toml: literal key: %s", err)
		}
	} else if r == '"' { // parse quoted key
		dec.read() // discard "
		var err error
		key, err = dec.parseQuotedString()
		if err != nil {
			return "", fmt.Errorf("toml: quoted key: %s", err)
		}
		if len(key) == 0 {
			return "", fmt.Errorf("toml: key cannot be empty")
		}
	} else { // parse bare key
		growingString := ""
		for {
			r := dec.peek()
			if isValidBareKeyChar(r) {
				dec.read()
				growingString += string(r)
			} else {
				break
			}
		}
		key = growingString
	}
	return key, nil
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

func (dec *Decoder) parseQuotedString() (string, error) {
	// assumes " has already been read
	// parses escape characters
	// does not accept new lines (or any unescaped control char)
	// reads the last "
	growingString := ""

	for {
		r := dec.peek()
		if r == eof {
			return "", fmt.Errorf("unifnished string")
		} else if r == '"' {
			dec.read()
			return growingString, nil
		} else if r == '\\' {
			dec.read() // read the \ char
			e := dec.peek()
			if e == eof {
				return "", fmt.Errorf("unfishied escape sequence")
			}

			if e == '"' || e == '\\' || e == '/' || e == 'b' {
				growingString += string(e)
			} else if e == 'b' {
				growingString += "\b"
			} else if e == 'f' {
				growingString += "\f"
			} else if e == 'n' {
				growingString += "\n"
			} else if e == 'r' {
				growingString += "\r"
			} else if e == 't' {
				growingString += "\t"
			} else if e == 'u' {
				dec.read() // read the u
				unicodeString, err := dec.parseUnicodeEscapeSequence(4)
				if err != nil {
					return "", fmt.Errorf("invalid 4-char unicode escape sequence: %s", err)
				}
				growingString += unicodeString
			} else if e == 'U' {
				dec.read() // read the U
				unicodeString, err := dec.parseUnicodeEscapeSequence(8)
				if err != nil {
					return "", fmt.Errorf("invalid 8-char unicode escape sequence: %s", err)
				}
				growingString += unicodeString
			}
		} else if 0x00 <= r && r <= 0x1F {
			return "", fmt.Errorf("unescaped control character %U", r)
		} else {
			dec.read()
			growingString += string(r)
		}
	}
}

func (dec *Decoder) parseUnicodeEscapeSequence(length int) (string, error) {
	hexRunes := dec.peekRunes(length)
	if len(hexRunes) < length || hexRunes[length - 1] == eof {
		return "", fmt.Errorf("unfinished sequence")
	}
	for _, c := range hexRunes {
		dec.read()
		if !isHexDigit(c) {
			return "", fmt.Errorf("incorrect character %c", c)
		}
	}
	code := string(hexRunes)
	intcode, err := strconv.ParseInt(code, 16, length * 8)
	if err != nil {
		return "", fmt.Errorf("invalid: \\u%s", code)
	}
	return string(rune(intcode)), nil
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

func isDigit(r rune) bool {
	return unicode.IsNumber(r)
}

func isHexDigit(r rune) bool {
	return isDigit(r) ||
		(r >= 'a' && r <= 'f') ||
		(r >= 'A' && r <= 'F')
}

func isAlphanumeric(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isValidBareKeyChar(r rune) bool {
	return isAlphanumeric(r) || r == '-' || unicode.IsNumber(r)
}