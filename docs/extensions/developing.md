# Writing an extension

An extension is any executable that speaks the [wire protocol](specification.md#5-wire-protocol).
It can be written in any language. This guide builds the smallest useful one in
Go with only the standard library; the full demonstration extension is
[`examples/extensions/relationship-digest`](../../examples/extensions/relationship-digest/).

## 1. A complete working example

`~/.config/tidecontacts/extensions/hello-count/extension.toml`:

```toml
id = "hello-count"
name = "Hello Count"
version = "0.1.0"
api = 1
description = "Counts people. Read-only."
command = "hello-count"
requires = ["core.contacts"]
permissions = ["contacts.read", "commands.register"]

[[commands]]
id = "count"
title = "Count people"
```

`main.go` (build with `go build -o hello-count .` and place the binary beside the manifest):

```go
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
)

type msg struct {
	API       int             `json:"api"`
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Method    string          `json:"method,omitempty"`
	OK        bool            `json:"ok,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     *struct{ Code, Message string } `json:"error,omitempty"`
}

func main() {
	in, out := bufio.NewReader(os.Stdin), json.NewEncoder(os.Stdout)
	line, _ := in.ReadBytes('\n')
	var req msg
	if json.Unmarshal(line, &req) != nil || req.Type != "request" {
		os.Exit(2)
	}
	reply := msg{API: 1, Type: "response", RequestID: req.RequestID, OK: true, Data: json.RawMessage(`{}`)}
	switch req.Method {
	case "command.run":
		// Ask the host for people. The host checks contacts.read before answering.
		args, _ := json.Marshal(map[string]any{"limit": 500})
		_ = out.Encode(msg{API: 1, Type: "host_call", CallID: "1", Method: "contacts.list", Data: args})
		line, _ = in.ReadBytes('\n')
		var r msg
		_ = json.Unmarshal(line, &r)
		var people []json.RawMessage
		if !r.OK {
			reply = msg{API: 1, Type: "response", RequestID: req.RequestID, Error: &struct{ Code, Message string }{"failed", "host refused: " + r.Error.Message}}
			break
		}
		_ = json.Unmarshal(r.Data, &people)
		reply.Data, _ = json.Marshal(map[string]any{"title": "People", "lines": []map[string]string{{"label": "Count", "text": fmt.Sprint(len(people))}}})
	case "health":
		reply.Data = json.RawMessage(`{"ok":true}`)
	}
	_ = out.Encode(reply)
}
```

Try it:

```sh
tidecontacts extensions list
tidecontacts extensions permissions hello-count
tidecontacts extensions enable hello-count --yes
tidecontacts extensions run hello-count count
```

## 2. Rules of the road

* **Answer every request with exactly one `response`.** Respond to unknown
  methods with `ok:false`; the host only calls the methods in the specification.
* **Read one line, write lines.** Flush each message. Don't print logs to stdout;
  stderr is fine (the host keeps and sanitizes the first 8 KiB for diagnostics).
* **Be quick.** Each request has a 10 s budget (hard cap 30 s). Long work should
  be split, or its result precomputed into `storage.*`.
* **You are stateless between calls.** Keep state in `storage.*`; keep settings
  in `[[config]]` and read them with `config.get`.
* **Ask for the least.** Every permission is shown to the user. Requesting more
  in an update forces re-approval and the extension stops running until then.
* **Expect refusals.** `permission_denied`, `conflict` (stale revision),
  `invalid_input` and `quota_exceeded` are normal answers, not crashes.
* **Return content, not terminal output.** Titles, labels and lines are plain
  text. ANSI sequences and control characters are stripped.
* **Events are hints.** They carry ids, not data, and can be dropped. Re-read
  what you need through the host API.
* **Don't write loops.** Writes you make carry `source: extension:<id>` and are
  never delivered to extensions, so you cannot trigger yourself or other extensions.

## 3. Writing safely

* Treat everything from the host (record text, names) as untrusted data when you
  build output or storage keys; never pass it to a shell.
* `contacts.get` returns the whole contact, including notes. Request only
  `contacts.read` if you only read; the host cannot stop a same-user process from
  also doing other things, so keep your own conduct to what you declared. See
  [security.md](security.md).

## 4. Testing

* **Unit-test the protocol handler** by piping a request line to your binary.
* **Use the real host:** `tidecontacts --demo extensions …` runs against the
  fictional demo data (set `TIDECONTACTS_EXTENSIONS_DIR` to a scratch directory
  for isolation). `examples/…` plus `scripts/extensions_smoke.py` is a worked
  end-to-end test; `internal/cli/extensions_test.go` is the Go equivalent.
* **Failure drills:** `internal/ext/ext_test.go` shows shell-script extensions
  that crash, hang, flood, print garbage and oversized output; copy the pattern
  to prove yours degrades cleanly.

## 5. Distribution notes

Phase 3A has no installer, registry or signature verification. Users install an
extension by copying its directory. The host records the executable's SHA-256 at
approval and asks again if it changes. Extensions are code that runs as the user:
distribute them with the same care as any program. If you intend to distribute a
commercial extension, read [licensing-boundary.md](licensing-boundary.md) first.
