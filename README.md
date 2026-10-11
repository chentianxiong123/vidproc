# vidproc

**vidproc** is a cross-platform command-line tool for lossless MP4 concatenation, metadata manipulation, and MP4 box inspection/injection. Designed for **general use** — no vendor assumptions, no file-naming conventions, pure parameter composition.

## Features

- **probe** — Extract full metadata (streams + format tags) as JSON
- **concat** — Lossless concatenation (`-c copy`) with optional stream compatibility verification
- **metadata** — Read/write standard format tags (creation_time, location, etc.)
- **boxes** — Inspect, extract, inject, or prune MP4 top-level boxes (free, udta, uuid, etc.)
- **timestamp** — Copy file timestamps (mtime, atime) from reference file
- **workflow** — One-shot composition: concat + metadata + boxes + timestamp

All operations are **lossless** (no re-encoding unless you explicitly re-encode). Original files are never modified — output goes to the path you specify.

## Installation

### Pre-built Binaries
Download the latest release for your platform from [Releases](https://github.com/example/vidproc/releases).

| Platform | Architectures |
|----------|---------------|
| Windows  | amd64         |
| Linux    | amd64, arm64  |
| macOS    | amd64, arm64  |

### Build from Source
```bash
git clone https://github.com/example/vidproc
cd vidproc
go build -o vidproc .
```

## Dependencies

**vidproc requires `ffmpeg` and `ffprobe` on your PATH.**  
Install from [ffmpeg.org](https://ffmpeg.org/download.html) or use your package manager:

```bash
# Linux
apt install ffmpeg        # Debian/Ubuntu
dnf install ffmpeg        # Fedora
pacman -S ffmpeg          # Arch

# macOS
brew install ffmpeg

# Windows (Chocolatey)
choco install ffmpeg

# Windows (Scoop)
scoop install ffmpeg
```

You can override the paths with `--ffmpeg` / `--ffprobe` flags.

## Quick Start

```bash
# 1. Probe a file
vidproc probe video.mp4

# 2. Losslessly concatenate multiple clips (verifies compatibility)
vidproc concat clip1.mp4 clip2.mp4 clip3.mp4 -o merged.mp4

# 3. Copy ALL metadata (including unknown boxes) from first clip to merged
vidproc metadata merged.mp4 merged_fixed.mp4 --copy-from clip1.mp4 --copy-unknown-boxes

# 4. One-shot workflow: concat + copy metadata + copy unknown boxes + copy timestamps
vidproc workflow clip*.mp4 -o final.mp4 \
  --metadata-from clip1.mp4 \
  --copy-unknown-boxes \
  --copy-timestamp-from clip1.mp4
```

## Command Reference

### Global Flags
```
--ffmpeg <path>      Path to ffmpeg binary (default: PATH lookup)
--ffprobe <path>     Path to ffprobe binary (default: PATH lookup)
--dry-run            Print commands without executing
--log-level <level>  error|warn|info|debug (default: info)
-v, --version        Print version and exit
-h, --help           Show help
```

### probe
```
vidproc probe [flags] <input...>
```
Flags:
```
--format json|text|table  Output format (default: json)
--show streams            Include stream info (default: true)
--show format             Include format tags (default: true)
--select-stream <spec>    Stream selector: v:0, a:0, etc. (default: all)
```
Outputs full stream info (codec, resolution, pix_fmt, sample rate, channels) + all format tags.

### concat
```
vidproc concat [flags] <input...> -o <output>
```
Flags:
```
-o, --output <file>   Output file (required)
--verify              Verify stream compatibility (default: true)
--no-verify           Skip compatibility check
--list-file <file>    Write concat list to file instead of temp
--force               Overwrite output without prompt
```
Verifies all inputs have matching video (codec, width, height, pix_fmt) and audio (codec, sample rate, channels) params.

### metadata
```
vidproc metadata [flags] <input> <output>
```
Flags:
```
--copy-from <file>    Copy ALL format tags from source file
--set key=value       Set specific tag (repeatable)
--delete key          Delete specific tag (repeatable)
--tags-file <file>    Read tags from JSON file
--strip-all           Remove all format tags before applying
--no-map              Don't use -map_metadata (use -metadata per tag)
```
Uses `ffmpeg -c copy` — no re-encoding. `--copy-from` is equivalent to `-map_metadata 1`.

### boxes
```
vidproc boxes <subcommand> [flags] <file>
```
Subcommands:
```
list                         List all top-level boxes (type, size, offset, standard/unknown)
extract <box-type> --output  Extract raw payloads of boxes matching type
inject <box-type> <src> -o   Inject boxes from src file into target
prune <box-type> -o          Remove boxes of type from file (not yet implemented)
```
Flags for extract/inject:
```
--match <substr>      Substring match on box payload
--dedup               Deduplicate by exact payload bytes
```

### timestamp
```
vidproc timestamp <target> <reference>
```
Copies mtime (and atime where supported) from reference to target.

### workflow
```
vidproc workflow [flags] <input...> -o <output>
```
Flags:
```
-o, --output <file>              Output file (required)
--metadata-from <file>           Copy metadata from this file (default: first input)
--copy-unknown-boxes             Copy all non-standard boxes (free, udta, uuid, etc.) from first input
--copy-timestamp-from <file>     Copy timestamps from this file (default: first input)
--no-verify                      Skip concat compatibility check
--force                          Overwrite output
```
Equivalent to: concat → metadata --copy-from --copy-unknown-boxes → timestamp

## Examples

### Batch process Xiaomi clips (preserve private tags)
```bash
vidproc workflow VID_20250209_*.mp4 -o merged.mp4 \
  --metadata-from VID_20250209_180531.mp4 \
  --copy-unknown-boxes \
  --copy-timestamp-from VID_20250209_180531.mp4
```

### Extract all private free-boxes from a file
```bash
vidproc boxes extract free source.mp4 --output free_boxes.bin
```

### Inject those free-boxes into another file
```bash
vidproc boxes inject free free_boxes.bin target.mp4 -o target_fixed.mp4
```

### Dry-run to preview commands
```bash
vidproc --dry-run workflow clip*.mp4 -o out.mp4 --metadata-from clip1.mp4
```

## Exit Codes
| Code | Meaning |
|------|---------|
| 0    | Success |
| 1    | Internal error |
| 2    | Usage error (bad flags, missing args) |
| 3    | Dependency missing (ffmpeg/ffprobe not found) |
| 4    | ffmpeg/ffprobe execution failed |
| 5    | MP4 parse error |

## Contributing

1. Fork the repo
2. Create a feature branch
3. Add tests for new functionality
4. Run `go test -v ./...`
5. Submit a PR

## License

MIT License — see [LICENSE](LICENSE) for details.

## Changelog

### 0.1.0 (2026-10-11)
- Initial release
- Commands: probe, concat, metadata, boxes, timestamp, workflow
- Cross-platform: Windows, Linux, macOS
- Pure Go MP4 box parser (no vendor logic)
- Full test coverage for core logic