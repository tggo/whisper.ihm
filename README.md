# whisper.ihm

Offline speech-to-text transcription tool for long audio files. Built with [whisper.cpp](https://github.com/ggml-org/whisper.cpp) and [ten-vad](https://github.com/TEN-framework/ten-vad) for voice activity detection.

## Features

- MP3 input with automatic resampling to 16kHz mono
- VAD-based segmentation — splits audio by silence, transcribes each chunk
- Accurate timestamps per segment
- Runs fully offline, no API keys required
- Metal GPU acceleration on macOS (Apple Silicon)

## Requirements

- macOS (Apple Silicon) or Linux
- Go 1.23+
- CMake
- Git

## Quick start

```bash
# Clone, build dependencies, download whisper model (~3 GB)
make setup

# Build the binary
make build

# Transcribe an MP3
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

## How it works

1. Decode MP3 to PCM, resample to 16kHz mono
2. Run VAD (ten-vad) to detect speech segments, split on ~500ms silence gaps
3. Feed each segment to whisper.cpp with timestamp offsets
4. Print `[start -> end] text` for each whisper segment

## License

MIT
