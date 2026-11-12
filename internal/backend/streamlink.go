package backend

import (
	"context"
	"fmt"
	"io"
	"time"

	processpkg "github.com/subculture-collective/twitchdl/internal/process"
)

// Streamlink resolves and records Twitch live streams.
type Streamlink struct {
	runner processpkg.Runner
	stdout io.Writer
	stderr io.Writer
}

// NewStreamlink constructs a Streamlink backend.
func NewStreamlink(runner processpkg.Runner, stdout, stderr io.Writer) *Streamlink {
	return &Streamlink{runner: runner, stdout: stdout, stderr: stderr}
}

// Probe returns nil when a requested live quality is currently available.
func (backend *Streamlink) Probe(ctx context.Context, target, quality string) error {
	if err := backend.runner.Run(ctx, processpkg.Spec{
		Name:   "streamlink",
		Args:   []string{"--stream-url", target, quality},
		Stdout: io.Discard,
		Stderr: io.Discard,
	}); err != nil {
		return fmt.Errorf("probe live stream: %w", err)
	}
	return nil
}

// Record records a currently available stream to a raw file.
func (backend *Streamlink) Record(ctx context.Context, target, quality, output string, duration time.Duration, fromStart bool) error {
	args := []string{"--output", output, "--force", "--progress", "yes"}
	if duration > 0 {
		args = append(args, "--stream-segmented-duration", duration.String())
	}
	if fromStart {
		args = append(args, "--hls-live-restart")
	}
	args = append(args, target, quality)
	if err := backend.runner.Run(ctx, processpkg.Spec{
		Name: "streamlink", Args: args, Stdout: backend.stdout, Stderr: backend.stderr, GracePeriod: 15 * time.Second,
	}); err != nil {
		return fmt.Errorf("record live stream: %w", err)
	}
	return nil
}
