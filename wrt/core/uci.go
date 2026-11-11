package main

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// uciSection is one `config <type> '<name>'` block of a UCI file.
type uciSection struct {
	Type    string
	Name    string
	Options map[string]string
	Lists   map[string][]string
}

func (s *uciSection) get(key, def string) string {
	if v, ok := s.Options[key]; ok {
		return v
	}
	return def
}

func (s *uciSection) bool(key string, def bool) bool {
	switch s.get(key, "") {
	case "1", "on", "true", "yes", "enabled":
		return true
	case "0", "off", "false", "no", "disabled":
		return false
	}
	return def
}

// parseUCI reads a UCI config file (as written by uci / LuCI). Anonymous sections get
// the name "@<type>[<index>]".
func parseUCI(r io.Reader) ([]*uciSection, error) {
	var out []*uciSection
	var cur *uciSection
	counts := map[string]int{}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 1<<20)
	for n := 1; sc.Scan(); n++ {
		toks, err := uciTokens(sc.Text())
		if err != nil {
			return nil, fmt.Errorf("line %d: %w", n, err)
		}
		if len(toks) == 0 {
			continue
		}
		switch toks[0] {
		case "package":
		case "config":
			if len(toks) < 2 {
				return nil, fmt.Errorf("line %d: config without type", n)
			}
			cur = &uciSection{Type: toks[1], Options: map[string]string{}, Lists: map[string][]string{}}
			if len(toks) > 2 && toks[2] != "" {
				cur.Name = toks[2]
			} else {
				cur.Name = fmt.Sprintf("@%s[%d]", toks[1], counts[toks[1]])
			}
			counts[toks[1]]++
			out = append(out, cur)
		case "option", "list":
			if cur == nil {
				return nil, fmt.Errorf("line %d: %s outside a section", n, toks[0])
			}
			if len(toks) < 2 {
				return nil, fmt.Errorf("line %d: %s without name", n, toks[0])
			}
			val := ""
			if len(toks) > 2 {
				val = toks[2]
			}
			if toks[0] == "option" {
				cur.Options[toks[1]] = val
			} else {
				cur.Lists[toks[1]] = append(cur.Lists[toks[1]], val)
			}
		default:
			return nil, fmt.Errorf("line %d: unknown keyword %q", n, toks[0])
		}
	}
	return out, sc.Err()
}

// uciTokens splits a line like libuci: words may be bare, 'single' or "double" quoted,
// adjacent pieces concatenate, and a backslash escapes the next character outside
// quotes (uci writes it's as three pieces: quoted it, an escaped quote, quoted s).
// A # outside quotes starts a comment.
func uciTokens(line string) ([]string, error) {
	var toks []string
	var b strings.Builder
	inTok := false
	flush := func() {
		if inTok {
			toks = append(toks, b.String())
			b.Reset()
			inTok = false
		}
	}
	for i := 0; i < len(line); i++ {
		c := line[i]
		switch {
		case c == ' ' || c == '\t' || c == '\r':
			flush()
		case c == '#' && !inTok:
			return toks, nil
		case c == '\'':
			inTok = true
			j := strings.IndexByte(line[i+1:], '\'')
			if j < 0 {
				return nil, fmt.Errorf("unterminated quote")
			}
			b.WriteString(line[i+1 : i+1+j])
			i += j + 1
		case c == '"':
			inTok = true
			i++
			for ; i < len(line) && line[i] != '"'; i++ {
				if line[i] == '\\' && i+1 < len(line) {
					i++
				}
				b.WriteByte(line[i])
			}
			if i >= len(line) {
				return nil, fmt.Errorf("unterminated quote")
			}
		case c == '\\' && i+1 < len(line):
			inTok = true
			i++
			b.WriteByte(line[i])
		default:
			inTok = true
			b.WriteByte(c)
		}
	}
	flush()
	return toks, nil
}
