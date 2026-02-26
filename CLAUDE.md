# Project: whisper.ihm

Offline audio transcription CLI tool.

## Architecture

- **main.go** — entry point, flags, audio loading, whisper processing loop
- **vad.go** — CGO wrapper for ten-vad (voice activity detection)
- **whisper_quiet.go** — suppresses whisper.cpp internal logging
- **Makefile** — builds whisper.cpp, downloads models, compiles Go binary

## Build

Requires CGO with whisper.cpp static libraries. The Makefile sets up all include/library paths via `CGO_ENV`. Run `make setup && make build`.

## Key patterns

- Audio pipeline: MP3 decode -> stereo-to-mono -> resample 16kHz -> VAD segmentation -> whisper per chunk
- VAD splits on ~500ms silence gaps with 200ms padding on each segment
- Whisper segments get absolute timestamps via offset from chunk start
- All stderr for progress, stdout for transcript output

## Dependencies

- whisper.cpp — cloned and built from source (git subdir, not committed)
- ten-vad — cloned (git subdir, not committed)
- Models downloaded to `models/` at setup time

## Release

- GitHub Actions workflow (`.github/workflows/release.yml`) triggers on tag push (`v*`)
- Builds macOS arm64 (Metal) and Linux amd64 (CPU-only)
- Uploads `tar.gz` + `sha256` to GitHub Releases
- Tag a release: `git tag v0.x.x && git push origin v0.x.x`

## Homebrew

- Tap repo: `tggo/homebrew-tap` (https://github.com/tggo/homebrew-tap)
- Formula: `Formula/whisper-ihm.rb`
- Install: `brew install tggo/tap/whisper-ihm`
- Builds from source on user's machine — no code signing needed
- On new release: update `url` and `sha256` in the formula

## Docker

- `Dockerfile` — multi-stage build, Linux CPU-only
- Builder: `golang:1.23-bookworm`, runtime: `debian:bookworm-slim`
- `docker build -t whisper-ihm .`

## Conventions

- Go 1.23, no external frameworks
- CGO for both whisper.cpp and ten-vad
- Flag-based CLI, no config files
- English only in code and comments
- `-trimpath` in go build to strip local paths from binary
