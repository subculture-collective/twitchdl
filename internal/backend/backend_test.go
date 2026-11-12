package backend

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/subculture-collective/twitchdl/internal/domain"
	processpkg "github.com/subculture-collective/twitchdl/internal/process"
)

type recordingRunner struct {
	specs []processpkg.Spec
	err   error
	hook  func(processpkg.Spec) error
}

func (runner *recordingRunner) Run(_ context.Context, spec processpkg.Spec) error {
	runner.specs = append(runner.specs, spec)
	if runner.hook != nil {
		return runner.hook(spec)
	}
	return runner.err
}

func TestYTDLPOpenEndedForcedDownload(t *testing.T) {
	t.Parallel()
	start := 5 * time.Second
	runner := &recordingRunner{}
	backend := NewYTDLP(runner, io.Discard, io.Discard)
	if err := backend.Download(context.Background(), domain.DownloadRequest{
		URL: "https://clips.twitch.tv/Test", Start: &start, OutputDir: t.TempDir(), Force: true,
	}); err != nil {
		t.Fatal(err)
	}
	args := strings.Join(runner.specs[0].Args, " ")
	if !strings.Contains(args, "*00:00:05-inf") || !strings.Contains(args, "--force-overwrites") {
		t.Fatalf("unexpected args: %s", args)
	}
}

func TestYTDLPDownloadArguments(t *testing.T) {
	t.Parallel()
	start := 90 * time.Second
	end := 2 * time.Minute
	runner := &recordingRunner{}
	backend := NewYTDLP(runner, io.Discard, io.Discard)
	err := backend.Download(context.Background(), domain.DownloadRequest{
		URL: "https://www.twitch.tv/videos/123", Start: &start, End: &end, OutputDir: t.TempDir(),
	})
	if err != nil {
		t.Fatalf("Download: %v", err)
	}
	if len(runner.specs) != 1 || runner.specs[0].Name != "yt-dlp" {
		t.Fatalf("unexpected process specs: %#v", runner.specs)
	}
	args := strings.Join(runner.specs[0].Args, " ")
	for _, want := range []string{"--no-playlist", "--no-overwrites", "--download-sections", "*00:01:30-00:02:00", "--force-keyframes-at-cuts"} {
		if !strings.Contains(args, want) {
			t.Errorf("arguments %q missing %q", args, want)
		}
	}
}

func TestProbeAndBackendErrors(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{err: errors.New("offline")}
	stream := NewStreamlink(runner, io.Discard, io.Discard)
	if err := stream.Probe(context.Background(), "https://www.twitch.tv/test", "best"); err == nil {
		t.Fatal("failed probe succeeded")
	}
	if err := stream.Record(context.Background(), "https://www.twitch.tv/test", "best", "test.part", 0, false); err == nil {
		t.Fatal("failed recording succeeded")
	}
}

func TestFinalizer(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	raw := filepath.Join(dir, "raw.part")
	staging := filepath.Join(dir, "final.mkv.finalizing")
	final := filepath.Join(dir, "final.mkv")
	if err := os.WriteFile(raw, []byte("raw"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &recordingRunner{hook: func(spec processpkg.Spec) error {
		if spec.Name == "ffmpeg" {
			return os.WriteFile(staging, []byte("final"), 0o600)
		}
		return nil
	}}
	finalizer := NewFinalizer(runner, io.Discard, io.Discard)
	if err := finalizer.Finalize(context.Background(), raw, staging, final, false); err != nil {
		t.Fatalf("Finalize: %v", err)
	}
	if _, err := os.Stat(final); err != nil {
		t.Fatalf("final file missing: %v", err)
	}
	if len(runner.specs) != 2 || runner.specs[0].Name != "ffmpeg" || runner.specs[1].Name != "ffprobe" {
		t.Fatalf("unexpected specs: %#v", runner.specs)
	}
	if err := finalizer.Finalize(context.Background(), raw, staging, final, false); err == nil {
		t.Fatal("existing final output accepted")
	}
}

func TestStreamlinkRecordArguments(t *testing.T) {
	t.Parallel()
	runner := &recordingRunner{}
	backend := NewStreamlink(runner, io.Discard, io.Discard)
	err := backend.Record(context.Background(), "https://www.twitch.tv/test", "720p60", "/tmp/test.part", 2*time.Hour, true)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	args := strings.Join(runner.specs[0].Args, " ")
	for _, want := range []string{"--output /tmp/test.part", "--stream-segmented-duration 2h0m0s", "--hls-live-restart", "720p60"} {
		if !strings.Contains(args, want) {
			t.Errorf("arguments %q missing %q", args, want)
		}
	}
}
