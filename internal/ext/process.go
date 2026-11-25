package ext

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

// Wire limits. All are enforced by the host; an extension cannot raise them.
const (
	maxLineBytes     = 1 << 20 // one protocol message
	maxTotalBytes    = 8 << 20 // all output of one invocation
	maxStderrBytes   = 8 << 10 // retained diagnostic text
	maxHostCalls     = 200     // host_call messages per invocation
	DefaultTimeout   = 10 * time.Second
	MaxTimeout       = 30 * time.Second
	killGrace        = 500 * time.Millisecond
	maxExecutableMiB = 256
)

// Error codes shared by the protocol and the CLI.
const (
	CodePermissionDenied = "permission_denied"
	CodeUnknownMethod    = "unknown_method"
	CodeInvalid          = "invalid_input"
	CodeNotFound         = "not_found"
	CodeConflict         = "conflict"
	CodeEntitlement      = "entitlement_required"
	CodeQuota            = "quota_exceeded"
	CodeInternal         = "internal"
)

// ProtocolError is an error reported by (or about) an extension. Its message is
// already sanitized.
type ProtocolError struct{ Code, Message string }

func (e *ProtocolError) Error() string { return e.Code + ": " + e.Message }

type message struct {
	API       int             `json:"api"`
	Type      string          `json:"type"`
	RequestID string          `json:"request_id,omitempty"`
	CallID    string          `json:"call_id,omitempty"`
	Method    string          `json:"method,omitempty"`
	OK        bool            `json:"ok,omitempty"`
	Data      json.RawMessage `json:"data,omitempty"`
	Error     *wireError      `json:"error,omitempty"`
}
type wireError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// HostFunc serves one host_call. It must enforce permissions itself.
type HostFunc func(ctx context.Context, method string, data json.RawMessage) (any, *ProtocolError)

// Result is the outcome of one invocation.
type Result struct {
	Data     json.RawMessage
	Duration time.Duration
	Stderr   string // sanitized, truncated diagnostic output
	HostCall int
}

// ResolveExecutable returns the real path of an extension's executable, proving
// it is a regular file that stays inside dir after symlinks are resolved.
func ResolveExecutable(dir, command string) (string, error) {
	if err := ValidateCommandPath(command); err != nil {
		return "", err
	}
	root, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return "", err
	}
	path, err := filepath.EvalSymlinks(filepath.Join(root, filepath.FromSlash(command)))
	if err != nil {
		return "", fmt.Errorf("executable: %w", err)
	}
	if rel, err := filepath.Rel(root, path); err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("executable escapes the extension directory")
	}
	st, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() {
		return "", fmt.Errorf("executable is not a regular file")
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o111 == 0 {
		return "", fmt.Errorf("executable is not marked executable")
	}
	if st.Size() > maxExecutableMiB<<20 {
		return "", fmt.Errorf("executable is larger than %d MiB", maxExecutableMiB)
	}
	return path, nil
}

// scrubbedEnv gives an extension the minimum it needs. No application secrets
// or inherited environment variables are passed. This is hygiene, not a sandbox.
func scrubbedEnv(id, dataDir string) []string {
	env := []string{"PATH=" + defaultPath(), "LANG=C.UTF-8", "TIDECONTACTS_EXTENSION_ID=" + id, fmt.Sprintf("TIDECONTACTS_EXTENSION_API=%d", APIVersion)}
	if dataDir != "" { // only extensions granted storage.sql learn where their data directory is
		env = append(env, "TIDECONTACTS_EXTENSION_DATA="+dataDir)
	}
	if runtime.GOOS == "windows" {
		env = append(env, "SystemRoot="+os.Getenv("SystemRoot"))
	}
	return env
}

func defaultPath() string {
	if runtime.GOOS == "windows" {
		return os.Getenv("SystemRoot") + `\System32`
	}
	return "/usr/bin:/bin"
}

type limitedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *limitedBuffer) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if room := maxStderrBytes - l.buf.Len(); room > 0 {
		l.buf.Write(p[:min(room, len(p))])
	}
	return len(p), nil // never block the child on diagnostics
}
func (l *limitedBuffer) String() string { l.mu.Lock(); defer l.mu.Unlock(); return l.buf.String() }

// Invoke runs the executable once: it writes one request, serves host_call
// messages through host, and returns the single response. The process is
// killed (with its process group on Unix) on timeout, cancellation, protocol
// violation or oversized output, and is always reaped before Invoke returns.
func Invoke(ctx context.Context, dir, command, id, method string, params any, timeout time.Duration, host HostFunc) (Result, error) {
	return InvokeOpts(ctx, dir, command, id, method, params, timeout, host, InvokeOptions{})
}

// InvokeOptions carries optional per-call settings.
type InvokeOptions struct {
	// DataDir, when set, is created (0700) and exposed as TIDECONTACTS_EXTENSION_DATA.
	DataDir string
}

// InvokeOpts is Invoke with options.
func InvokeOpts(ctx context.Context, dir, command, id, method string, params any, timeout time.Duration, host HostFunc, opt InvokeOptions) (Result, error) {
	start := time.Now()
	res := Result{}
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	timeout = min(timeout, MaxTimeout)
	exe, err := ResolveExecutable(dir, command)
	if err != nil {
		return res, err
	}
	if opt.DataDir != "" {
		if err := os.MkdirAll(opt.DataDir, 0o700); err != nil {
			return res, fmt.Errorf("extension data directory: %w", err)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	raw, err := json.Marshal(params)
	if err != nil {
		return res, err
	}
	reqID := NewID()
	first, _ := json.Marshal(message{API: APIVersion, Type: "request", RequestID: reqID, Method: method, Data: raw})

	cmd := exec.CommandContext(ctx, exe)
	cmd.Dir = dir
	cmd.Env = scrubbedEnv(id, opt.DataDir)
	setProcessGroup(cmd)
	cmd.Cancel = func() error { return killProcess(cmd) }
	cmd.WaitDelay = killGrace
	stderr := &limitedBuffer{}
	cmd.Stderr = stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return res, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return res, err
	}
	if err := cmd.Start(); err != nil {
		return res, fmt.Errorf("start extension: %w", err)
	}
	finish := func(e error) (Result, error) {
		cancel()
		_ = stdin.Close()
		_ = killProcess(cmd)
		_ = cmd.Wait()
		res.Duration = time.Since(start)
		res.Stderr = Clean(stderr.String(), 400, false)
		return res, e
	}
	if _, err := stdin.Write(append(first, '\n')); err != nil {
		return finish(fmt.Errorf("write request: %w", err))
	}
	r := bufio.NewReaderSize(io.LimitReader(stdout, maxTotalBytes), 64<<10)
	for {
		line, err := readLine(r)
		if err != nil {
			if ctx.Err() != nil {
				return finish(fmt.Errorf("extension timed out after %s", timeout))
			}
			if errors.Is(err, io.EOF) {
				return finish(fmt.Errorf("extension exited without a response"))
			}
			return finish(err)
		}
		var m message
		if err := json.Unmarshal(line, &m); err != nil {
			return finish(fmt.Errorf("protocol violation: malformed JSON message"))
		}
		if m.API != APIVersion {
			return finish(fmt.Errorf("protocol violation: unsupported api %d", m.API))
		}
		switch m.Type {
		case "response":
			if m.RequestID != reqID {
				return finish(fmt.Errorf("protocol violation: response for a different request"))
			}
			if !m.OK {
				e := &ProtocolError{Code: CodeInternal, Message: "extension reported an error"}
				if m.Error != nil {
					e.Code, e.Message = Clean(m.Error.Code, 40, false), Clean(m.Error.Message, 300, false)
				}
				res, _ = finish(nil)
				return res, e
			}
			res.Data = m.Data
			// Wait for a clean exit so the process is reaped, then report.
			_ = stdin.Close()
			out, _ := finish(nil)
			out.Data = m.Data
			return out, nil
		case "host_call":
			res.HostCall++
			if res.HostCall > maxHostCalls {
				return finish(fmt.Errorf("extension exceeded %d host calls", maxHostCalls))
			}
			var reply message
			reply.API, reply.Type, reply.CallID = APIVersion, "host_response", m.CallID
			if host == nil {
				reply.Error = &wireError{CodeUnknownMethod, "host API is not available"}
			} else if data, perr := host(ctx, m.Method, m.Data); perr != nil {
				reply.Error = &wireError{perr.Code, perr.Message}
			} else if b, merr := json.Marshal(data); merr != nil || len(b) > maxLineBytes {
				reply.Error = &wireError{CodeInvalid, "host response too large"}
			} else {
				reply.OK, reply.Data = true, b
			}
			out, _ := json.Marshal(reply)
			if _, err := stdin.Write(append(out, '\n')); err != nil {
				return finish(fmt.Errorf("extension closed its input: %w", err))
			}
		default:
			return finish(fmt.Errorf("protocol violation: unexpected message type %q", m.Type))
		}
	}
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var out []byte
	for {
		chunk, isPrefix, err := r.ReadLine()
		if err != nil {
			return nil, err
		}
		out = append(out, chunk...)
		if len(out) > maxLineBytes {
			return nil, fmt.Errorf("protocol violation: message exceeds %d bytes", maxLineBytes)
		}
		if !isPrefix {
			if len(bytes.TrimSpace(out)) == 0 {
				out = out[:0]
				continue
			}
			return out, nil
		}
	}
}
