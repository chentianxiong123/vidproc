# vidproc

**vidproc** is a cross-platform command-line tool for lossless MP4 concatenation, metadata manipulation, and MP4 box inspection/injection. Designed for **general use** — no vendor assumptions, no file-naming conventions, pure parameter composition.

## Features

**MP4 container**
- **probe** — Extract full metadata (streams + format tags) as JSON
- **concat** — Lossless concatenation (`-c copy`) with optional stream compatibility verification
- **metadata** — Read/write standard format tags (creation_time, location, etc.)
- **boxes** — Inspect, extract, inject, or prune MP4 top-level boxes (free, udta, uuid, etc.)
- **timestamp** — Copy file timestamps (mtime, atime) from reference file
- **workflow** — One-shot composition: concat + metadata + boxes + timestamp

**Video editing** — 13 atomic ops, each doing one thing:
- **crop**, **rotate**, **flip** — geometry
- **speed**, **volume**, **text** — filters
- **audio**, **replace-audio**, **title** — audio
- **thumbnail**, **gif**, **loop**, **split** — export

Atomic ops compose into **pipelines** (`vidproc run -c "a | b"`) and **YAML workflows**.

MP4 container operations are **lossless** (no re-encoding). Video editing ops re-encode where the change requires it, and stream-copy video whenever a filter only touches audio. Original files are never modified — output goes to the path you specify.

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


### Video Editing Operations

Beyond the MP4-level commands above, vidproc ships a set of **atomic video ops**. Each one does exactly one thing and takes a single input and output. They work standalone, in a **pipeline** (`|`), and in **YAML workflows** — new ops are automatically available in all three.

#### Geometry

```
crop -i <input> -o <output> -w <width> -h <height> [-x <left>] [-y <top>]
rotate -i <input> -o <output> -angle <90|180|270>
flip -i <input> -o <output> -dir <h|v>
```

| Op | Flags | Notes |
|----|-------|-------|
| `crop` | `-w`, `-h` (required); `-x`, `-y` | `-x`/`-y` default to centering. Re-encodes. |
| `rotate` | `-angle 90\|180\|270` (required) | Clockwise. 90/270 swap width and height. |
| `flip` | `-dir h\|v` (required) | `h` mirrors horizontally, `v` flips vertically. |

#### Filters

```
speed -i <input> -o <output> -rate <n> [-noaudio]
volume -i <input> -o <output> [-level <n>] [-mute]
text -i <input> -o <output> -text <s> [-x <n>] [-y <n>] [-font <path>] [-size <n>] \
     [-color <c>] [-box] [-boxcolor <c>] [-boxpad <n>] [-border <n>] [-bordercolor <c>] \
     [-start <t>] [-end <t>]
```

| Op | Flags | Notes |
|----|-------|-------|
| `speed` | `-rate` (required); `-noaudio` | `2` = 2x faster, `0.5` = half speed. Audio is time-stretched to match. |
| `volume` | `-level`, `-mute` | `2` = louder, `0.5` = quieter. Video is stream-copied — no re-encode. |
| `text` | `-text` (required); see usage | `-x`/`-y` default to centering and accept expressions. `-start`/`-end` gate the visible window. `-border`/`-boxpad` set outline and box padding. |

Text rendering handles `%`, `:`, `'`, `\`, newlines, and CJK without escaping — pass a non-Latin font via `-font` (defaults probe common system fonts).

#### Audio

```
audio -i <input> -o <output> [-codec copy|aac|mp3|opus|flac|pcm_s16le]
replace-audio -i <video> -o <output> -a <audio> [-start <t>]
title -o <output> -text <s> [-duration <t>] [-w <n>] [-h <n>] [-bg <color>] \
      [-color <color>] [-size <n>] [-font <path>]
```

| Op | Flags | Notes |
|----|-------|-------|
| `audio` | `-codec copy` (default) | Extracts audio only. Output format is inferred from `-o`'s extension. Use `-codec pcm_s16le` for WAV (`wav` is a container, not a codec). |
| `replace-audio` | `-a` (required); `-start` | Video stream is copied losslessly; only audio is muxed. `-start` delays the new track. |
| `title` | `-text` (required) | Generates a title card from scratch (default 1280x720, 3s). `-i` is accepted but ignored. Output is `concat`-compatible h264/aac. |

#### Export

```
thumbnail -i <input> -o <img> [-ss <t>] [-w <width>]
gif -i <input> -o <out.gif> [-ss <t>] [-t <seconds>] [-w <width>] [-fps <n>]
loop -i <input> -o <output> -times <n>
split -i <input> -o <prefix> -every <seconds> [-copy]
```

| Op | Flags | Notes |
|----|-------|-------|
| `thumbnail` | `-o` (required); `-ss`, `-w` | `-ss` defaults to `0`. `-w` scales height proportionally. Output `.jpg` or `.png`. |
| `gif` | `-o` (required) | Two-pass palette for stable colors. Defaults: width 480, 12 fps. |
| `loop` | `-times` | Total plays. `3` plays the clip 3 times; `1` is a passthrough. Stream-copied. |
| `split` | `-every` (required) | Writes `<prefix>0.mp4`, `1.mp4`, ... Default re-encodes so cuts land exactly on the boundary. |

**`split -copy` caveat:** stream copying cannot seek inside a GOP, so cuts snap to the nearest keyframe and segment lengths drift. Prefer the re-encoding default unless speed matters more than accuracy.

#### Pipelines and YAML

Ops compose with `|`. Each step runs on the previous step's output:

```bash
vidproc run -i in.mp4 -o out.mp4 -c "crop -w 320 -h 180 | flip -dir h"
vidproc run -i in.mp4 -o out.mp4 -c "speed -rate 1.5 | text -text 'Fast!' -x 10 -y 10"
```

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

### 0.2.0 (2026-10-11)
- 13 atomic video ops: `crop`, `rotate`, `flip`, `speed`, `volume`, `text`, `audio`, `replace-audio`, `title`, `thumbnail`, `gif`, `loop`, `split`
- Pipeline engine: compose ops with `vidproc run -c "op a | op b"`
- YAML workflows for repeatable multi-step jobs
- `--dry-run` prints the ffmpeg command without executing
- Preset-driven UX — no raw ffmpeg parameters exposed

### 0.1.0 (2026-10-11)
- Initial release
- Commands: probe, concat, metadata, boxes, timestamp, workflow
- Cross-platform: Windows, Linux, macOS
- Pure Go MP4 box parser (no vendor logic)
- Full test coverage for core logic