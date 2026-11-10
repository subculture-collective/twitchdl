# twitchdl

`twitchdl` is a command-line tool that downloads public Twitch clips and VODs
and records public live streams. It is written in Go and drives `yt-dlp`,
Streamlink, `ffmpeg` and `ffprobe` to do the work.

It is an early version. It handles public URLs and channel names only; Twitch
API discovery, scheduled recording, signed-in content and subscriber-only media
are not supported.

> Download only media you are permitted to save. You are responsible for
> complying with copyright law and Twitch's terms.

## Features

- Download a clip or VOD, whole or between two timestamps.
- Record a live stream until you press Ctrl+C, for a set duration, or after
  waiting for the channel to go live.
- Choose the Streamlink quality, the output directory, and MKV (default) or MP4
  output.
- Recordings are written to a `.part` file first, then remuxed without
  re-encoding and checked with `ffprobe` before the final file appears. If
  finalizing fails, the raw recording is kept.
- `twitchdl doctor` checks that the media tools are installed.
- Shell completion for bash, zsh and fish.

## Requirements

- Go 1.26 or newer to build
- `yt-dlp`
- `streamlink`
- `ffmpeg` and `ffprobe`

## Quick start

```bash
make build
./bin/twitchdl doctor

# Clip, whole or from 0:05 to 0:20
./bin/twitchdl clip 'https://clips.twitch.tv/CLIP_SLUG'
./bin/twitchdl clip 'https://clips.twitch.tv/CLIP_SLUG' 5 20

# Whole or ranged VOD
./bin/twitchdl vod 'https://www.twitch.tv/videos/1234567890'
./bin/twitchdl vod 'https://www.twitch.tv/videos/1234567890' 1:20:00 1:35:00

# Record now until Ctrl+C
./bin/twitchdl live twitchdev

# Wait for the channel to go live, indefinitely or for up to 6 hours
./bin/twitchdl live twitchdev --wait
./bin/twitchdl live twitchdev --wait-timeout 6h

# Record two hours as MP4
./bin/twitchdl live twitchdev --duration 2h --container mp4
```

Timestamps accept seconds, `MM:SS` or `HH:MM:SS`, with optional milliseconds.
Every command takes `--output-dir` and `--force` (allow replacing a file).
`live` also takes `--quality`, `--from-start` and `--keep-raw`. See
[docs/usage.md](docs/usage.md) and [docs/live-recording.md](docs/live-recording.md).

To install the binary and bash completion under `~/.local`:

```bash
./scripts/install.sh
```

## Development

```bash
make verify        # format check, vet, race tests, 80% coverage floor, build, smoke test
make integration   # records a generated local HLS stream with the real media tools
```

The test suite runs offline with fake backends and never contacts Twitch.
`make help` lists every target.

```text
cmd/twitchdl/      entry point
internal/app/      media orchestration
internal/backend/  yt-dlp and Streamlink adapters and the ffmpeg finalizer
internal/cli/      command-line interface
internal/domain/   shared request and result types
internal/media/    URL parsing, timestamps and output paths
internal/process/  process execution
```

More: [architecture](docs/architecture.md),
[troubleshooting](docs/troubleshooting.md) and
[design decisions](docs/adr/README.md).

## Contributing

- Report bugs and request features in
  [GitHub issues](https://github.com/subculture-collective/twitchdl/issues).
- Send code as a pull request on GitHub against `main`. See
  [CONTRIBUTING.md](CONTRIBUTING.md); report security problems as described in
  [SECURITY.md](SECURITY.md).

CI installs the media tools and runs `make verify` on every pull request. The
`CI gate` check must pass before a pull request can merge.

## License

GPL-3.0-only. See [LICENSE](LICENSE).
