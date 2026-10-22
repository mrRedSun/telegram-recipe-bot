package youtube

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os/exec"
	"strings"
)

type Diagnostic struct {
	Stage      string
	Backend    string
	Source     string
	Outcome    string
	Reason     string
	ExitCode   int
	HTTPStatus int
	Items      int
	Bytes      int64
	ElapsedMS  int64
}

type commandError struct {
	Stage, Backend, Reason string
	ExitCode, HTTPStatus   int
}

func (e *commandError) Error() string {
	return "source command failed (" + e.Stage + ": " + e.Reason + ")"
}
func diagnose(err error) Diagnostic {
	d := Diagnostic{Reason: "internal"}
	var command *commandError
	if errors.As(err, &command) {
		d.Stage = command.Stage
		d.Backend = command.Backend
		d.Reason = command.Reason
		d.ExitCode = command.ExitCode
		d.HTTPStatus = command.HTTPStatus
	}
	var unsupported *UnsupportedError
	if errors.As(err, &unsupported) {
		d.Reason = "unsupported_source"
	}
	if errors.Is(err, context.DeadlineExceeded) {
		d.Reason = "timeout"
	}
	if errors.Is(err, context.Canceled) {
		d.Reason = "cancelled"
	}
	return d
}

type boundedBuffer struct {
	bytes.Buffer
	Limit    int
	Overflow bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	n := len(p)
	remaining := b.Limit - b.Len()
	if len(p) > remaining {
		b.Overflow = true
		if remaining > 0 {
			_, _ = b.Buffer.Write(p[:remaining])
		}
	} else {
		_, _ = b.Buffer.Write(p)
	}
	return n, nil
}
func runBounded(ctx context.Context, stage, tool string, args []string, limit int) ([]byte, error) {
	cmd := exec.CommandContext(ctx, tool, args...)
	stdout := &boundedBuffer{Limit: limit}
	stderr := &boundedBuffer{Limit: 64 << 10}
	cmd.Stdout, cmd.Stderr = stdout, stderr
	err := cmd.Run()
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if stdout.Overflow {
		return nil, &commandError{Stage: stage, Backend: tool, Reason: "output_limit"}
	}
	if err != nil {
		reason, code, status := "command_failed", -1, 0
		if errors.Is(err, exec.ErrNotFound) {
			reason = "command_missing"
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			code = exit.ExitCode()
			reason, status = classifyStderr(stderr.String())
		}
		return nil, &commandError{Stage: stage, Backend: tool, Reason: reason, ExitCode: code, HTTPStatus: status}
	}
	return io.ReadAll(bytes.NewReader(stdout.Bytes()))
}
func classifyStderr(stderr string) (string, int) {
	s := strings.ToLower(stderr)
	switch {
	case strings.Contains(s, "429"), strings.Contains(s, "rate limit"), strings.Contains(s, "too many requests"):
		return "rate_limited", 429
	case strings.Contains(s, "403"), strings.Contains(s, "forbidden"):
		return "forbidden", 403
	case strings.Contains(s, "login"), strings.Contains(s, "log in"), strings.Contains(s, "authentication"), strings.Contains(s, "cookies"):
		return "login_required", 0
	case strings.Contains(s, "404"), strings.Contains(s, "not found"), strings.Contains(s, "unavailable"):
		return "unavailable", 0
	case strings.Contains(s, "timed out"), strings.Contains(s, "timeout"):
		return "network_timeout", 0
	default:
		return "command_failed", 0
	}
}
