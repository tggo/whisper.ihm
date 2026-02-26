# whisper.ihm

Offline speech-to-text transcription tool for long audio files. Built with [whisper.cpp](https://github.com/ggml-org/whisper.cpp) and [ten-vad](https://github.com/TEN-framework/ten-vad) for voice activity detection.

## Features

- MP3 input with automatic resampling to 16kHz mono
- VAD-based segmentation — splits audio by silence, transcribes each chunk
- Accurate timestamps per segment
- Runs fully offline, no API keys required
- Metal GPU acceleration on macOS (Apple Silicon)

## Install

### Homebrew (macOS)

```bash
brew install tggo/tap/whisper-ihm
```

### From source

Requires Go 1.23+, CMake, Git.

```bash
git clone https://github.com/tggo/whisper.ihm.git && cd whisper.ihm
make setup   # clones deps, builds whisper.cpp, downloads model (~3 GB)
make build   # compiles the binary
./whisper-ihm recording.mp3
```

## Usage

```
Usage: whisper-ihm [flags] <input.mp3>

Flags:
  -model string    Path to GGML model (default "models/ggml-large-v3.bin")
  -lang string     Language code (default "auto")
  -threads int     Number of threads (default: all CPUs)
  -help            Show help
```

## Output format

```
[00:00:01.200 -> 00:00:05.800] Hello, how are you today?
[00:00:06.100 -> 00:00:09.400] I'm doing well, thank you.
```

## Install from release

Download a pre-built binary from [Releases](https://github.com/tggo/whisper.ihm/releases):

```bash
# macOS (Apple Silicon)
curl -L https://github.com/tggo/whisper.ihm/releases/latest/download/whisper-ihm-darwin-arm64.tar.gz | tar xz

# Linux (amd64)
curl -L https://github.com/tggo/whisper.ihm/releases/latest/download/whisper-ihm-linux-amd64.tar.gz | tar xz

# Download the whisper model (~3 GB)
mkdir -p models
curl -L -o models/ggml-large-v3.bin \
  https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin

./whisper-ihm recording.mp3
```

## Docker

```bash
# Build the image (Linux, CPU-only)
docker build -t whisper-ihm .

# Download model and transcribe
docker run -v $(pwd)/data:/data whisper-ihm -model /data/ggml-large-v3.bin /data/recording.mp3
```

The Dockerfile uses a multi-stage build: `golang:1.23-bookworm` for building (clones whisper.cpp + ten-vad, compiles with CGO), `debian:bookworm-slim` for runtime.

## Build details

- `-trimpath` strips local filesystem paths from the binary
- macOS builds include Metal GPU acceleration
- Linux/Docker builds are CPU-only

## How it works

1. Decode MP3 to PCM, resample to 16kHz mono
2. Run VAD (ten-vad) to detect speech segments, split on ~500ms silence gaps
3. Feed each segment to whisper.cpp with timestamp offsets
4. Print `[start -> end] text` for each whisper segment

## License

MIT
