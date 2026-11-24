package forgeobserve

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"unicode"
)

const (
	maxName    = 128
	maxHelp    = 256
	maxLabel   = 128
	maxValue   = 256
	maxSeries  = 32
	maxMessage = 256
)

var ErrInvalid = errors.New("observation is not valid")

func validName(name string) error {
	if name == "" || len(name) > maxName {
		return fmt.Errorf("%w: name is not valid", ErrInvalid)
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_', r == ':':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return fmt.Errorf("%w: name is not valid", ErrInvalid)
		}
	}
	return nil
}

func validLabel(name string) error {
	if name == "" || len(name) > maxLabel || strings.Contains(name, ":") {
		return fmt.Errorf("%w: label is not valid", ErrInvalid)
	}
	for i, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r == '_':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return fmt.Errorf("%w: label is not valid", ErrInvalid)
		}
	}
	return nil
}

func validHelp(help string) error {
	if strings.TrimSpace(help) == "" || len(help) > maxHelp || strings.ContainsAny(help, "\r") {
		return fmt.Errorf("%w: help is not valid", ErrInvalid)
	}
	return nil
}

func labelsOf(kv []string) (string, error) {
	if len(kv) == 0 || len(kv)%2 != 0 {
		return "", fmt.Errorf("%w: labels are not valid", ErrInvalid)
	}
	type pair struct{ key, val string }
	pairs := make([]pair, 0, len(kv)/2)
	seen := map[string]bool{}
	for i := 0; i < len(kv); i += 2 {
		key, val := kv[i], kv[i+1]
		if err := validLabel(key); err != nil {
			return "", err
		}
		if seen[key] {
			return "", fmt.Errorf("%w: duplicate label %s", ErrInvalid, key)
		}
		if len(val) > maxValue {
			return "", fmt.Errorf("%w: label value is too long", ErrInvalid)
		}
		seen[key] = true
		pairs = append(pairs, pair{key, escape(val, true)})
	}
	sort.Slice(pairs, func(i, j int) bool { return pairs[i].key < pairs[j].key })
	var b strings.Builder
	b.WriteByte('{')
	for i, p := range pairs {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(p.key)
		b.WriteString(`="`)
		b.WriteString(p.val)
		b.WriteByte('"')
	}
	b.WriteByte('}')
	return b.String(), nil
}

func escape(s string, quoted bool) string {
	var b strings.Builder
	for _, r := range s {
		switch r {
		case '\\':
			b.WriteString(`\\`)
		case '\n':
			b.WriteString(`\n`)
		case '"':
			if quoted {
				b.WriteString(`\"`)
			} else {
				b.WriteRune(r)
			}
		default:
			if unicode.IsControl(r) {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
				continue
			}
			b.WriteRune(r)
		}
	}
	return b.String()
}
