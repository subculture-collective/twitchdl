package backend

import (
	"context"
	"fmt"
	"io"
	"os"

	processpkg "github.com/subculture-collective/twitchdl/internal/process"
)

// Finalizer remuxes and validates raw recordings.
type Finalizer struct {
	runner processpkg.Runner
	stdout io.Writer
	stderr io.Writer
}

// NewFinalizer constructs an ffmpeg/ffprobe finalizer.
func NewFinalizer(runner processpkg.Runner, stdout, stderr io.Writer) *Finalizer {
	return &Finalizer{runner: runner, stdout: stdout, stderr: stderr}
}

// Finalize remuxes raw media into staging, validates it, and publishes it atomically.
func (finalizer *Finalizer) Finalize(ctx context.Context, rawPath, stagingPath, finalPath string, force bool) error {
	if !force {
		if _, err := os.Stat(finalPath); err == nil {
			return fmt.Errorf("output already exists: %s", finalPath)
		} else if !os.IsNotExist(err) {
			return fmt.Errorf("inspect final output: %w", err)
		}
	}
	ffmpegArgs := []string{"-hide_banner", "-nostdin", "-loglevel", "warning"}
	if force {
		ffmpegArgs = append(ffmpegArgs, "-y")
	} else {
		ffmpegArgs = append(ffmpegArgs, "-n")
	}
	ffmpegArgs = append(ffmpegArgs, "-i", rawPath, "-map", "0", "-c", "copy", stagingPath)
	if err := finalizer.runner.Run(ctx, processpkg.Spec{
		Name: "ffmpeg", Args: ffmpegArgs, Stdout: finalizer.stdout, Stderr: finalizer.stderr,
	}); err != nil {
		return fmt.Errorf("remux recording; raw file retained at %s: %w", rawPath, err)
	}
	if err := finalizer.runner.Run(ctx, processpkg.Spec{
		Name:   "ffprobe",
		Args:   []string{"-v", "error", "-show_entries", "format=duration", "-of", "default=noprint_wrappers=1:nokey=1", stagingPath},
		Stdout: io.Discard, Stderr: finalizer.stderr,
	}); err != nil {
		return fmt.Errorf("validate recording; raw file retained at %s: %w", rawPath, err)
	}
	if force {
		if err := os.Remove(finalPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("replace existing output: %w", err)
		}
	}
	if err := os.Rename(stagingPath, finalPath); err != nil {
		return fmt.Errorf("publish finalized recording; raw file retained at %s: %w", rawPath, err)
	}
	return nil
}
