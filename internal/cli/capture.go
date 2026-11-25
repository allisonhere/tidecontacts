package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/mail"
	"strings"
	"unicode"

	"github.com/allisonhere/tidecontacts/internal/storage/sqlite"
)

func captureSender(ctx context.Context, s *sqlite.Store, in io.Reader, out io.Writer, asJSON bool) error {
	// Limit header parsing independently of attachments. Some clients include an mbox envelope.
	header := &io.LimitedReader{R: in, N: 1 << 20}
	reader := bufio.NewReader(header)
	if prefix, _ := reader.Peek(5); string(prefix) == "From " {
		if _, err := reader.ReadString('\n'); err != nil {
			return invalid("invalid mbox envelope")
		}
	}
	msg, err := mail.ReadMessage(reader)
	if err != nil || header.N == 0 {
		return invalid("invalid or oversized message header")
	}
	if len(msg.Header["From"]) != 1 {
		return invalid("expected one From header")
	}
	from, err := mail.ParseAddress(msg.Header.Get("From"))
	if err != nil || strings.IndexFunc(from.Address, func(r rune) bool { return unicode.IsControl(r) || unicode.Is(unicode.Cf, r) }) >= 0 {
		return invalid("expected one valid From address")
	}
	from.Name = singleLine(from.Name)
	if _, err = io.Copy(io.Discard, in); err != nil {
		return err
	}
	id, created, err := s.CaptureSender(ctx, from)
	if err != nil {
		return err
	}
	status := "Already saved"
	if created {
		status = "Saved"
	}
	if asJSON {
		return json.NewEncoder(out).Encode(envelope{Version: 1, Data: map[string]any{"contact_id": id, "created": created, "email": from.Address}})
	}
	_, err = fmt.Fprintf(out, "%s: %s\n", status, from.Address)
	return err
}
