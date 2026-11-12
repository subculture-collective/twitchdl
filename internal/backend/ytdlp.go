// Package backend adapts mature media tools to twitchdl application contracts.
package backend

import (
	"context"
	"fmt"
	"io"

	"github.com/subculture-collective/twitchdl/internal/domain"
	"github.com/subculture-collective/twitchdl/internal/media"
	processpkg "github.com/subculture-collective/twitchdl/internal/process"
)

// YTDLP downloads Twitch clips and VODs.
type YTDLP struct {
	runner processpkg.Runner
	stdout io.Writer
	stderr io.Writer
}

// NewYTDLP constructs a yt-dlp backend.
func NewYTDLP(runner processpkg.Runner, stdout, stderr io.Writer) *YTDLP {
	return &YTDLP{runner: runner, stdout: stdout, stderr: stderr}
}

// Download downloads a clip or VOD request.
func (backend *YTDLP) Download(ctx context.Context, request domain.DownloadRequest) error {
	outputDir, err := media.EnsureOutputDir(request.OutputDir)
	if err != nil {
		return err
	}
	args := []string{
		"--no-playlist",
		"--newline",
		"--embed-metadata",
		"--embed-thumbnail",
		"--paths", outputDir,
		"--output", "%(uploader)s - %(title)s [%(id)s].%(ext)s",
	}
	if request.Force {
		args = append(args, "--force-overwrites")
	} else {
		args = append(args, "--no-overwrites")
	}
	if request.Start != nil {
		end := "inf"
		if request.End != nil {
			end = media.FormatTimestamp(*request.End)
		}
		section := fmt.Sprintf("*%s-%s", media.FormatTimestamp(*request.Start), end)
		args = append(args, "--download-sections", section, "--force-keyframes-at-cuts")
	}
	args = append(args, request.URL)
	if err := backend.runner.Run(ctx, processpkg.Spec{
		Name: "yt-dlp", Args: args, Stdout: backend.stdout, Stderr: backend.stderr,
	}); err != nil {
		return fmt.Errorf("download media: %w", err)
	}
	return nil
}
