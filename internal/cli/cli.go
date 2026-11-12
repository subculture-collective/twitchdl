// Package cli adapts Cobra commands to the UI-independent application service.
package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/subculture-collective/twitchdl/internal/app"
	"github.com/subculture-collective/twitchdl/internal/backend"
	"github.com/subculture-collective/twitchdl/internal/domain"
	"github.com/subculture-collective/twitchdl/internal/media"
	processpkg "github.com/subculture-collective/twitchdl/internal/process"
)

// Version is overridden at build time for releases.
var Version = "0.1.0-dev"

type terminalReporter struct {
	writer io.Writer
}

func (reporter terminalReporter) Report(event domain.Event) {
	if _, err := fmt.Fprintf(reporter.writer, "[%s] %s\n", event.Kind, event.Message); err != nil {
		return
	}
}

// New constructs the twitchdl root command and all subcommands.
func New(stdout, stderr io.Writer) *cobra.Command {
	runner := processpkg.OSRunner{}
	reporter := terminalReporter{writer: stderr}
	service := app.NewService(
		backend.NewYTDLP(runner, stdout, stderr),
		backend.NewStreamlink(runner, stdout, stderr),
		backend.NewFinalizer(runner, stdout, stderr),
		reporter,
	)

	root := &cobra.Command{
		Use:           "twitchdl",
		Short:         "Download Twitch clips, VODs, and live streams",
		Version:       Version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.SetOut(stdout)
	root.SetErr(stderr)
	root.AddCommand(newDownloadCommand("clip", service.DownloadClip))
	root.AddCommand(newDownloadCommand("vod", service.DownloadVOD))
	root.AddCommand(newLiveCommand(service))
	root.AddCommand(newDoctorCommand(stdout))
	root.AddCommand(newCompletionCommand(root))
	return root
}

func newDownloadCommand(name string, run func(context.Context, domain.DownloadRequest) (domain.Result, error)) *cobra.Command {
	var outputDir string
	var force bool
	command := &cobra.Command{
		Use:   name + " URL [START [END]]",
		Short: "Download a Twitch " + name,
		Args:  cobra.RangeArgs(1, 3),
		RunE: func(command *cobra.Command, args []string) error {
			start, end, err := parseRange(args[1:])
			if err != nil {
				return err
			}
			result, err := run(command.Context(), domain.DownloadRequest{
				URL: args[0], Start: start, End: end, OutputDir: outputDir, Force: force,
			})
			if err != nil {
				return err
			}
			if _, err := fmt.Fprintln(command.OutOrStdout(), result.Path); err != nil {
				return fmt.Errorf("write result: %w", err)
			}
			return nil
		},
	}
	command.Flags().StringVarP(&outputDir, "output-dir", "o", ".", "destination directory")
	command.Flags().BoolVar(&force, "force", false, "allow replacing existing output")
	return command
}

func newLiveCommand(service *app.Service) *cobra.Command {
	var request domain.LiveRequest
	var container string
	command := &cobra.Command{
		Use:   "live CHANNEL_OR_URL",
		Short: "Record a currently running Twitch stream",
		Args:  cobra.ExactArgs(1),
		RunE: func(command *cobra.Command, args []string) error {
			request.Target = args[0]
			request.Container = domain.Container(container)
			result, err := service.RecordLive(command.Context(), request)
			if result.Path != "" {
				if _, writeErr := fmt.Fprintln(command.OutOrStdout(), result.Path); writeErr != nil {
					return fmt.Errorf("write result: %w", writeErr)
				}
			}
			return err
		},
	}
	flags := command.Flags()
	flags.StringVarP(&request.OutputDir, "output-dir", "o", ".", "destination directory")
	flags.StringVar(&request.Quality, "quality", "best", "Streamlink quality selection")
	flags.StringVar(&container, "container", "mkv", "final container: mkv or mp4")
	flags.DurationVar(&request.Duration, "duration", 0, "stop after recording for this duration")
	flags.BoolVar(&request.Wait, "wait", false, "wait indefinitely for the channel to go live")
	flags.DurationVar(&request.WaitTimeout, "wait-timeout", 0, "wait up to this duration for the channel to go live")
	flags.BoolVar(&request.FromStart, "from-start", false, "start at the earliest retained live segment")
	flags.BoolVar(&request.KeepRaw, "keep-raw", false, "retain Streamlink's raw recording after finalization")
	flags.BoolVar(&request.Force, "force", false, "allow replacing existing output")
	return command
}

func newDoctorCommand(stdout io.Writer) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check external media dependencies",
		Args:  cobra.NoArgs,
		RunE: func(command *cobra.Command, _ []string) error {
			var missing []string
			for _, status := range app.Doctor(command.Context()) {
				if status.Error != nil {
					if _, err := fmt.Fprintf(stdout, "%-10s missing (%v)\n", status.Name, status.Error); err != nil {
						return fmt.Errorf("write doctor result: %w", err)
					}
					missing = append(missing, status.Name)
					continue
				}
				if _, err := fmt.Fprintf(stdout, "%-10s %s [%s]\n", status.Name, status.Version, status.Path); err != nil {
					return fmt.Errorf("write doctor result: %w", err)
				}
			}
			if len(missing) > 0 {
				return fmt.Errorf("missing required tools: %v; on Arch run: sudo pacman -S yt-dlp streamlink ffmpeg", missing)
			}
			return nil
		},
	}
}

func newCompletionCommand(root *cobra.Command) *cobra.Command {
	command := &cobra.Command{Use: "completion", Short: "Generate shell completion", Args: cobra.ExactArgs(1)}
	command.RunE = func(command *cobra.Command, args []string) error {
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(command.OutOrStdout())
		case "zsh":
			return root.GenZshCompletion(command.OutOrStdout())
		case "fish":
			return root.GenFishCompletion(command.OutOrStdout(), true)
		default:
			return fmt.Errorf("unsupported shell %q: use bash, zsh, or fish", args[0])
		}
	}
	return command
}

func parseRange(args []string) (*time.Duration, *time.Duration, error) {
	if len(args) == 0 {
		return nil, nil, nil
	}
	start, err := media.ParseTimestamp(args[0])
	if err != nil {
		return nil, nil, err
	}
	if len(args) == 1 {
		return &start, nil, nil
	}
	end, err := media.ParseTimestamp(args[1])
	if err != nil {
		return nil, nil, err
	}
	if err := media.ValidateRange(&start, &end); err != nil {
		return nil, nil, err
	}
	return &start, &end, nil
}

// ExitCode maps command errors to stable process exit codes.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	if errors.Is(err, domain.ErrInterrupted) {
		return 130
	}
	return 1
}

// Execute runs the CLI with process standard streams.
func Execute() int {
	command := New(os.Stdout, os.Stderr)
	if err := command.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "twitchdl: error:", err)
		return ExitCode(err)
	}
	return 0
}
