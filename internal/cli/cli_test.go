package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/subculture-collective/twitchdl/internal/domain"
)

func TestRootCommandOfflineBehavior(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		args     []string
		wantErr  bool
		contains string
	}{
		{name: "help", args: []string{"--help"}, contains: "Download Twitch clips"},
		{name: "version", args: []string{"--version"}, contains: Version},
		{name: "invalid clip", args: []string{"clip", "https://example.com/no"}, wantErr: true},
		{name: "reversed VOD range", args: []string{"vod", "https://www.twitch.tv/videos/123", "2:00", "1:00"}, wantErr: true},
		{name: "mutually exclusive wait", args: []string{"live", "channel", "--wait", "--wait-timeout", "1h"}, wantErr: true},
		{name: "bash completion", args: []string{"completion", "bash"}, contains: "__start_twitchdl"},
		{name: "bad completion", args: []string{"completion", "powershell"}, wantErr: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			var stdout bytes.Buffer
			var stderr bytes.Buffer
			command := New(&stdout, &stderr)
			command.SetArgs(test.args)
			err := command.ExecuteContext(context.Background())
			if test.wantErr && err == nil {
				t.Fatal("expected error")
			}
			if !test.wantErr && err != nil {
				t.Fatalf("Execute: %v", err)
			}
			if test.contains != "" && !strings.Contains(stdout.String(), test.contains) {
				t.Fatalf("stdout=%q, want %q", stdout.String(), test.contains)
			}
		})
	}
}

func TestParseRangeAndExitCode(t *testing.T) {
	t.Parallel()
	start, end, err := parseRange([]string{"1:30", "2:00"})
	if err != nil || start == nil || end == nil || *start != 90*time.Second || *end != 2*time.Minute {
		t.Fatalf("parseRange got (%v, %v, %v)", start, end, err)
	}
	if _, _, err := parseRange([]string{"bad"}); err == nil {
		t.Fatal("invalid range accepted")
	}
	if got := ExitCode(nil); got != 0 {
		t.Errorf("ExitCode(nil)=%d", got)
	}
	if got := ExitCode(errors.New("failure")); got != 1 {
		t.Errorf("ExitCode(failure)=%d", got)
	}
	if got := ExitCode(domain.ErrInterrupted); got != 130 {
		t.Errorf("ExitCode(interrupted)=%d", got)
	}
}

type failedWriter struct{}

func (failedWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestTerminalReporterHandlesClosedWriter(t *testing.T) {
	t.Parallel()
	terminalReporter{writer: failedWriter{}}.Report(domain.Event{Kind: domain.EventWarning, Message: "test"})
}
