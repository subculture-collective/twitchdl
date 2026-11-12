// Package app implements twitchdl use cases independently of any UI.
package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/subculture-collective/twitchdl/internal/domain"
	"github.com/subculture-collective/twitchdl/internal/media"
)

// Downloader downloads clips and VODs.
type Downloader interface {
	Download(context.Context, domain.DownloadRequest) error
}

// LiveBackend probes and records live streams.
type LiveBackend interface {
	Probe(context.Context, string, string) error
	Record(context.Context, string, string, string, time.Duration, bool) error
}

// Finalizer publishes a validated recording.
type Finalizer interface {
	Finalize(context.Context, string, string, string, bool) error
}

// Service coordinates media backends and recoverable output handling.
type Service struct {
	downloader      Downloader
	live            LiveBackend
	finalizer       Finalizer
	reporter        domain.Reporter
	now             func() time.Time
	probeInterval   time.Duration
	finalizeTimeout time.Duration
}

// NewService constructs the application service.
func NewService(downloader Downloader, live LiveBackend, finalizer Finalizer, reporter domain.Reporter) *Service {
	if reporter == nil {
		reporter = domain.NopReporter{}
	}
	return &Service{
		downloader: downloader, live: live, finalizer: finalizer, reporter: reporter,
		now: time.Now, probeInterval: 15 * time.Second, finalizeTimeout: 2 * time.Minute,
	}
}

// DownloadClip validates and downloads a Twitch clip.
func (service *Service) DownloadClip(ctx context.Context, request domain.DownloadRequest) (domain.Result, error) {
	if err := media.ValidateClipURL(request.URL); err != nil {
		return domain.Result{}, err
	}
	return service.download(ctx, request, "clip")
}

// DownloadVOD validates and downloads a Twitch VOD.
func (service *Service) DownloadVOD(ctx context.Context, request domain.DownloadRequest) (domain.Result, error) {
	if err := media.ValidateVODURL(request.URL); err != nil {
		return domain.Result{}, err
	}
	return service.download(ctx, request, "VOD")
}

func (service *Service) download(ctx context.Context, request domain.DownloadRequest, kind string) (domain.Result, error) {
	if service.downloader == nil {
		return domain.Result{}, fmt.Errorf("%s downloader is not configured", kind)
	}
	if err := media.ValidateRange(request.Start, request.End); err != nil {
		return domain.Result{}, err
	}
	if request.OutputDir == "" {
		request.OutputDir = "."
	}
	service.reporter.Report(domain.Event{Kind: domain.EventResolving, Message: "Downloading Twitch " + kind})
	if err := service.downloader.Download(ctx, request); err != nil {
		return domain.Result{}, err
	}
	service.reporter.Report(domain.Event{Kind: domain.EventComplete, Message: "Download complete in " + request.OutputDir})
	return domain.Result{Path: request.OutputDir}, nil
}

// RecordLive waits as requested, records a live stream, and finalizes recoverable output.
func (service *Service) RecordLive(ctx context.Context, request domain.LiveRequest) (domain.Result, error) {
	if service.live == nil || service.finalizer == nil {
		return domain.Result{}, fmt.Errorf("live recording backends are not configured")
	}
	if request.Wait && request.WaitTimeout > 0 {
		return domain.Result{}, fmt.Errorf("--wait and --wait-timeout are mutually exclusive")
	}
	if request.Duration < 0 || request.WaitTimeout < 0 {
		return domain.Result{}, fmt.Errorf("durations cannot be negative")
	}
	if request.Quality == "" {
		request.Quality = "best"
	}
	if request.Container == "" {
		request.Container = domain.ContainerMKV
	}
	if err := domain.ValidateContainer(request.Container); err != nil {
		return domain.Result{}, err
	}
	target, channel, err := media.NormalizeLiveTarget(request.Target)
	if err != nil {
		return domain.Result{}, err
	}
	if request.OutputDir == "" {
		request.OutputDir = "."
	}
	outputDir, err := media.EnsureOutputDir(request.OutputDir)
	if err != nil {
		return domain.Result{}, err
	}

	service.reporter.Report(domain.Event{Kind: domain.EventResolving, Message: "Checking whether " + channel + " is live"})
	if err := service.waitUntilLive(ctx, target, request.Quality, request.Wait, request.WaitTimeout); err != nil {
		return domain.Result{}, err
	}

	rawPath, stagingPath, finalPath := media.LivePaths(outputDir, channel, string(request.Container), service.now())
	if !request.Force {
		for _, path := range []string{rawPath, stagingPath, finalPath} {
			if _, statErr := os.Stat(path); statErr == nil {
				return domain.Result{}, fmt.Errorf("output already exists: %s", path)
			} else if !os.IsNotExist(statErr) {
				return domain.Result{}, fmt.Errorf("inspect output %s: %w", path, statErr)
			}
		}
	}

	service.reporter.Report(domain.Event{Kind: domain.EventRecording, Message: "Recording " + channel + " to " + rawPath})
	recordErr := service.live.Record(ctx, target, request.Quality, rawPath, request.Duration, request.FromStart)
	info, statErr := os.Stat(rawPath)
	if statErr != nil || info.Size() == 0 {
		if recordErr != nil {
			return domain.Result{RawPath: rawPath}, recordErr
		}
		if statErr != nil {
			return domain.Result{RawPath: rawPath}, fmt.Errorf("recording produced no raw file: %w", statErr)
		}
		return domain.Result{RawPath: rawPath}, fmt.Errorf("recording produced an empty raw file: %s", rawPath)
	}

	service.reporter.Report(domain.Event{Kind: domain.EventFinalizing, Message: "Finalizing " + finalPath})
	finalizeCtx, cancel := context.WithTimeout(context.Background(), service.finalizeTimeout)
	defer cancel()
	if err := service.finalizer.Finalize(finalizeCtx, rawPath, stagingPath, finalPath, request.Force); err != nil {
		return domain.Result{RawPath: rawPath}, err
	}
	if !request.KeepRaw {
		if err := os.Remove(rawPath); err != nil {
			service.reporter.Report(domain.Event{Kind: domain.EventWarning, Message: "Final output is valid, but raw file could not be removed: " + err.Error()})
		}
	}
	result := domain.Result{Path: finalPath, RawPath: rawPath, Interrupted: ctx.Err() != nil}
	service.reporter.Report(domain.Event{Kind: domain.EventComplete, Message: "Saved " + finalPath})
	if recordErr != nil {
		if ctx.Err() != nil || errors.Is(recordErr, context.Canceled) {
			return result, fmt.Errorf("%w: %v", domain.ErrInterrupted, recordErr)
		}
		return result, fmt.Errorf("recording finalized after recorder error: %w", recordErr)
	}
	return result, nil
}

func (service *Service) waitUntilLive(ctx context.Context, target, quality string, wait bool, waitTimeout time.Duration) error {
	if err := service.live.Probe(ctx, target, quality); err == nil {
		return nil
	} else if !wait && waitTimeout == 0 {
		return fmt.Errorf("channel is offline or quality %q is unavailable: %w", quality, err)
	}

	waitCtx := ctx
	cancel := func() {}
	if waitTimeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, waitTimeout)
	}
	defer cancel()
	service.reporter.Report(domain.Event{Kind: domain.EventWaiting, Message: "Channel is offline; waiting for a live stream"})
	ticker := time.NewTicker(service.probeInterval)
	defer ticker.Stop()
	for {
		select {
		case <-waitCtx.Done():
			if ctx.Err() != nil {
				return fmt.Errorf("waiting interrupted: %w", ctx.Err())
			}
			return fmt.Errorf("channel did not go live within %s: %w", waitTimeout, waitCtx.Err())
		case <-ticker.C:
			if err := service.live.Probe(waitCtx, target, quality); err == nil {
				return nil
			}
		}
	}
}
