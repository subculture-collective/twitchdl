package app

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/subculture-collective/twitchdl/internal/domain"
)

type fakeDownloader struct {
	requests []domain.DownloadRequest
	err      error
}

func (fake *fakeDownloader) Download(_ context.Context, request domain.DownloadRequest) error {
	fake.requests = append(fake.requests, request)
	return fake.err
}

type fakeLive struct {
	probeFailures int
	probes        int
	recordErr     error
	skipWrite     bool
	emptyWrite    bool
}

func (fake *fakeLive) Probe(_ context.Context, _, _ string) error {
	fake.probes++
	if fake.probes <= fake.probeFailures {
		return errors.New("offline")
	}
	return nil
}

func (fake *fakeLive) Record(_ context.Context, _, _, output string, _ time.Duration, _ bool) error {
	if fake.skipWrite {
		return fake.recordErr
	}
	content := []byte("raw-media")
	if fake.emptyWrite {
		content = nil
	}
	if err := os.WriteFile(output, content, 0o600); err != nil {
		return err
	}
	return fake.recordErr
}

type fakeFinalizer struct{}

func (fakeFinalizer) Finalize(_ context.Context, _, _, final string, _ bool) error {
	return os.WriteFile(final, []byte("final-media"), 0o600)
}

type failingFinalizer struct{}

func (failingFinalizer) Finalize(context.Context, string, string, string, bool) error {
	return errors.New("finalization failed")
}

func TestDownloadValidationAndDelegation(t *testing.T) {
	t.Parallel()
	downloader := &fakeDownloader{}
	service := NewService(downloader, nil, nil, nil)
	if _, err := service.DownloadClip(context.Background(), domain.DownloadRequest{URL: "https://example.com/no"}); err == nil {
		t.Fatal("invalid clip URL accepted")
	}
	if _, err := service.DownloadVOD(context.Background(), domain.DownloadRequest{URL: "https://www.twitch.tv/videos/123"}); err != nil {
		t.Fatalf("valid VOD rejected: %v", err)
	}
	if len(downloader.requests) != 1 {
		t.Fatalf("got %d downloads, want 1", len(downloader.requests))
	}
}

func TestDownloadErrors(t *testing.T) {
	t.Parallel()
	service := NewService(nil, nil, nil, nil)
	if _, err := service.DownloadClip(context.Background(), domain.DownloadRequest{URL: "https://clips.twitch.tv/Test"}); err == nil {
		t.Fatal("missing downloader accepted")
	}
	downloader := &fakeDownloader{err: errors.New("download failed")}
	service = NewService(downloader, nil, nil, nil)
	start := 2 * time.Minute
	end := time.Minute
	if _, err := service.DownloadVOD(context.Background(), domain.DownloadRequest{
		URL: "https://www.twitch.tv/videos/123", Start: &start, End: &end,
	}); err == nil {
		t.Fatal("reversed range accepted")
	}
	if _, err := service.DownloadClip(context.Background(), domain.DownloadRequest{URL: "https://clips.twitch.tv/Test"}); err == nil {
		t.Fatal("downloader error suppressed")
	}
}

func TestRecordLiveFinalizesOutput(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	service := NewService(nil, &fakeLive{}, fakeFinalizer{}, nil)
	service.now = func() time.Time { return time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC) }
	result, err := service.RecordLive(context.Background(), domain.LiveRequest{Target: "testchannel", OutputDir: dir})
	if err != nil {
		t.Fatalf("RecordLive: %v", err)
	}
	if filepath.Ext(result.Path) != ".mkv" {
		t.Fatalf("final path %q does not use MKV", result.Path)
	}
	if _, err := os.Stat(result.Path); err != nil {
		t.Fatalf("final output missing: %v", err)
	}
	if _, err := os.Stat(result.RawPath); !os.IsNotExist(err) {
		t.Fatalf("raw file should be removed, stat error=%v", err)
	}
}

func TestRecordLiveInterruptedStillFinalizes(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	service := NewService(nil, &fakeLive{recordErr: context.Canceled}, fakeFinalizer{}, nil)
	result, err := service.RecordLive(ctx, domain.LiveRequest{Target: "testchannel", OutputDir: dir, KeepRaw: true})
	if !errors.Is(err, domain.ErrInterrupted) {
		t.Fatalf("error=%v, want ErrInterrupted", err)
	}
	if result.Path == "" || !result.Interrupted {
		t.Fatalf("unexpected result: %#v", result)
	}
}

func TestRecordLiveWaitTimeout(t *testing.T) {
	t.Parallel()
	service := NewService(nil, &fakeLive{probeFailures: 100}, fakeFinalizer{}, nil)
	service.probeInterval = time.Millisecond
	_, err := service.RecordLive(context.Background(), domain.LiveRequest{
		Target: "testchannel", OutputDir: t.TempDir(), WaitTimeout: 3 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("wait timeout succeeded")
	}
}

func TestRecordLiveValidationErrors(t *testing.T) {
	t.Parallel()
	service := NewService(nil, &fakeLive{}, fakeFinalizer{}, nil)
	tests := []domain.LiveRequest{
		{Target: "channel", Wait: true, WaitTimeout: time.Minute},
		{Target: "channel", Duration: -time.Second},
		{Target: "channel", Container: "avi"},
		{Target: "https://example.com/channel"},
	}
	for _, request := range tests {
		if _, err := service.RecordLive(context.Background(), request); err == nil {
			t.Errorf("request %#v succeeded", request)
		}
	}
	if _, err := NewService(nil, nil, nil, nil).RecordLive(context.Background(), domain.LiveRequest{Target: "channel"}); err == nil {
		t.Fatal("missing live backends accepted")
	}
}

func TestRecordLiveFailureRecovery(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name      string
		live      *fakeLive
		finalizer Finalizer
	}{
		{name: "no raw file", live: &fakeLive{skipWrite: true, recordErr: errors.New("capture failed")}, finalizer: fakeFinalizer{}},
		{name: "empty raw file", live: &fakeLive{emptyWrite: true}, finalizer: fakeFinalizer{}},
		{name: "finalizer failure", live: &fakeLive{}, finalizer: failingFinalizer{}},
		{name: "recorder error after data", live: &fakeLive{recordErr: errors.New("network failed")}, finalizer: fakeFinalizer{}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			service := NewService(nil, test.live, test.finalizer, nil)
			result, err := service.RecordLive(context.Background(), domain.LiveRequest{
				Target: "channel", OutputDir: t.TempDir(), KeepRaw: true,
			})
			if err == nil {
				t.Fatal("expected error")
			}
			if test.name == "finalizer failure" {
				if _, statErr := os.Stat(result.RawPath); statErr != nil {
					t.Fatalf("raw file not retained: %v", statErr)
				}
			}
		})
	}
}

func TestRecordLiveWaitsThenRecords(t *testing.T) {
	t.Parallel()
	live := &fakeLive{probeFailures: 2}
	service := NewService(nil, live, fakeFinalizer{}, nil)
	service.probeInterval = time.Millisecond
	if _, err := service.RecordLive(context.Background(), domain.LiveRequest{
		Target: "channel", OutputDir: t.TempDir(), Wait: true,
	}); err != nil {
		t.Fatalf("RecordLive: %v", err)
	}
	if live.probes != 3 {
		t.Fatalf("probes=%d, want 3", live.probes)
	}
}

func TestDoctorShape(t *testing.T) {
	t.Parallel()
	statuses := Doctor(context.Background())
	if len(statuses) != 4 {
		t.Fatalf("Doctor returned %d tools, want 4", len(statuses))
	}
	for _, status := range statuses {
		if status.Name == "" {
			t.Fatal("doctor returned unnamed tool")
		}
	}
}
