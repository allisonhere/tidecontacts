package transfer

import (
	"bufio"
	"bytes"
	"fmt"
	"net/mail"
	"strconv"
	"strings"
	"unicode"

	"github.com/allisonhere/tidecontacts/internal/domain"
)

// ReadMuttFile reads alias definitions as data. It never evaluates configuration,
// expands variables, executes commands, or follows source directives.
func ReadMuttFile(path string) (File, error) {
	data, err := readLocalFile(path)
	if err != nil {
		return File{}, err
	}
	return ReadMutt(data)
}

func ReadMutt(data []byte) (File, error) {
	out := File{Format: "mutt"}
	scan := bufio.NewScanner(bytes.NewReader(bytes.TrimPrefix(data, []byte{239, 187, 191})))
	scan.Buffer(make([]byte, 4096), 50<<20)
	logical := ""
	line, start := 0, 1
	for scan.Scan() {
		line++
		if logical == "" {
			start = line
		}
		part := strings.TrimSuffix(scan.Text(), "\r")
		slashes := len(part) - len(strings.TrimRight(part, "\\"))
		if slashes%2 == 1 {
			logical += strings.TrimSuffix(part, "\\") + " "
			continue
		}
		logical += part
		contacts, err := parseMuttAlias(logical)
		if err != nil {
			return File{}, fmt.Errorf("line %d: %w", start, err)
		}
		out.Cards = append(out.Cards, contacts...)
		logical = ""
	}
	if err := scan.Err(); err != nil {
		return File{}, err
	}
	if logical != "" {
		return File{}, fmt.Errorf("line %d: unfinished continuation", start)
	}
	if len(out.Cards) == 0 {
		return File{}, fmt.Errorf("no alias definitions found; choose the file containing your alias lines")
	}
	return out, nil
}

func muttWord(s string) (word, rest string, err error) {
	s = strings.TrimSpace(s)
	var b strings.Builder
	var quote rune
	escaped := false
	for i, r := range s {
		if escaped {
			b.WriteRune(r)
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				b.WriteRune(r)
			}
			continue
		}
		if r == '\'' || r == '"' {
			quote = r
			continue
		}
		if unicode.IsSpace(r) {
			return b.String(), strings.TrimSpace(s[i:]), nil
		}
		b.WriteRune(r)
	}
	if quote != 0 || escaped {
		return "", "", fmt.Errorf("unfinished quote or escape")
	}
	return b.String(), "", nil
}

func parseMuttAlias(line string) ([]domain.Contact, error) {
	command, rest, err := muttWord(line)
	if err != nil {
		return nil, err
	}
	if command == "unalias" {
		return nil, fmt.Errorf("unalias directives are not supported; choose a file containing current alias definitions")
	}
	if command != "alias" {
		return nil, nil
	}
	if strings.ContainsAny(rest, "`$") {
		return nil, fmt.Errorf("dynamic aliases are unsupported; use literal email addresses")
	}
	groups := []string{}
	key, rest, err := muttWord(rest)
	if err != nil {
		return nil, err
	}
	for key == "-group" {
		group, remaining, e := muttWord(rest)
		if e != nil || group == "" {
			return nil, fmt.Errorf("missing alias group")
		}
		groups = append(groups, group)
		key, rest, err = muttWord(remaining)
		if err != nil {
			return nil, err
		}
	}
	if key == "" || rest == "" {
		return nil, fmt.Errorf("alias needs a name and email address")
	}
	addresses, comment, err := muttAddresses(rest)
	if err != nil {
		return nil, err
	}
	parsed, err := mail.ParseAddressList(addresses)
	if err != nil {
		// Mutt also allows quoting the entire address argument.
		if len(addresses) >= 2 && addresses[0] == '\'' && addresses[len(addresses)-1] == '\'' {
			addresses = addresses[1 : len(addresses)-1]
		} else if decoded, e := strconv.Unquote(addresses); e == nil {
			addresses = decoded
		}
		parsed, err = mail.ParseAddressList(addresses)
	}
	if err != nil {
		return nil, fmt.Errorf("alias %q: invalid address list (alias references are unsupported): %w", key, err)
	}
	result := []domain.Contact{}
	for _, address := range parsed {
		if !strings.Contains(address.Address, "@") {
			return nil, fmt.Errorf("alias %q: use literal email addresses", key)
		}
		c := domain.NewContact("person")
		c.DisplayName = address.Name
		if c.DisplayName == "" {
			c.DisplayName = key
		}
		c.Nickname = key
		c.Notes = comment
		c.Tags = append([]string(nil), groups...)
		c.Methods = []domain.Method{{Kind: "email", Value: address.Address, Preferred: true, Source: "mutt"}}
		c.Normalize()
		result = append(result, c)
	}
	return result, nil
}

// Locate configuration comments without splitting quoted display names or RFC comments.
func muttAddresses(s string) (string, string, error) {
	var quote rune
	escaped := false
	depth := 0
	for i, r := range s {
		if escaped {
			escaped = false
			continue
		}
		if r == '\\' {
			escaped = true
			continue
		}
		if quote != 0 {
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			continue
		}
		if r == '(' {
			depth++
		}
		if r == ')' && depth > 0 {
			depth--
		}
		if depth == 0 && r == '#' {
			return strings.TrimSpace(s[:i]), strings.TrimSpace(s[i+1:]), nil
		}
		if depth == 0 && r == ';' {
			return "", "", fmt.Errorf("multiple commands on an alias line are unsupported")
		}
	}
	if quote != 0 || escaped {
		return "", "", fmt.Errorf("unfinished address quote or escape")
	}
	return strings.TrimSpace(s), "", nil
}
