// Package cmdrender 渲染纯函数门:CLI/Web/AI 三嘴共用的唯一渲染入口。
//
// 垂直切片纪律:所有渲染能力从这里出,不外露 ffmpeg 细节。
// 输入 project.yaml 路径,输出渲染结果;dry-run 输出 filtergraph。
package cmdrender

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"vidproc/internal/render/composer"
	"vidproc/internal/render/presets"
	"vidproc/internal/timeline/spec"
)

// Result 渲染结果。
type Result struct {
	Path     string
	Size     int64
	Duration time.Duration
	Elapsed  time.Duration
}

// Options 渲染选项。
type Options struct {
	// Output 覆盖 spec 中的输出路径。
	Output string
	// DryRun 只编译 filtergraph,不执行 ffmpeg。
	DryRun bool
	// GPU 硬件加速名字(nvenc 等),空 = spec 默认。
	GPU string
	// Logger 自定义日志,空 = slog.Default()。
	Logger *slog.Logger
	// OnProgress 渲染进度回调(每 ~0.5s 触发一次,含百分比/帧/speed)。
	// 空 = 不报告。Web/CLI 进度条都从这拿数。
	OnProgress composer.ProgressHandler
}

// Filtergraph 编译结果(dry-run 用)。
type Filtergraph struct {
	// Args 完整 ffmpeg 命令行(含 filter_complex)。
	Args []string
	// Nodes DAG 节点数。
	Nodes int
}

func resolvePlatformPreset(presetName string) (presets.PlatformPreset, error) {
	switch strings.ToLower(strings.TrimSpace(presetName)) {
	case "tiktok", "tiktok_1080p60", "tiktok_vertical":
		return presets.TikTokVertical1080p60(), nil
	case "instagram_reels", "reels":
		return presets.InstagramReels1080p30(), nil
	case "instagram_square", "square":
		return presets.InstagramSquare1080p(), nil
	case "youtube_4k", "youtube_4k60":
		return presets.YouTube4K60(), nil
	case "youtube_1080", "youtube_1080p60":
		return presets.YouTube1080p60(), nil
	case "web_fast", "web_720p30":
		return presets.WebFast720p30(), nil
	default:
		return presets.PlatformPreset{}, fmt.Errorf("unknown platform preset %q", presetName)
	}
}

// Compile 把 project.yaml 编译成 filtergraph(不执行 ffmpeg)。
func Compile(ctx context.Context, specPath string, opts Options) (*Filtergraph, error) {
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	specification, err := spec.ParseFile(specPath)
	if err != nil {
		return nil, err
	}
	spec.ResolveAssetPaths(specification, filepath.Dir(specPath))
	if opts.Output != "" {
		specification.Output = opts.Output
	}
	if opts.GPU != "" {
		specification.HardwareAcceleration = opts.GPU
	}

	compositionTimeline, encodingOptions, finalOutputPath, err := spec.ToTimeline(specification)
	if err != nil {
		return nil, err
	}

	composerOptions := []composer.Option{composer.WithLogger(logger)}
	if encodingOptions != nil {
		composerOptions = append(composerOptions, composer.WithEncoding(*encodingOptions))
	}
	if specification.Preset != "" {
		if resolvedPreset, presetErr := resolvePlatformPreset(specification.Preset); presetErr == nil {
			composerOptions = append(composerOptions, composer.WithPreset(resolvedPreset))
		}
	}
	if specification.HardwareAcceleration != "" && strings.ToLower(specification.HardwareAcceleration) != "none" {
		accelerator := presets.HardwareAccelerator(strings.ToLower(specification.HardwareAcceleration))
		composerOptions = append(composerOptions, composer.WithHardwareAcceleration(accelerator, false))
	}

	composerInstance := composer.New(composerOptions...)
	compilationResult, err := composerInstance.Compile(compositionTimeline, finalOutputPath)
	if err != nil {
		return nil, err
	}

	nodeCount := 0
	for range compilationResult.Graph.Nodes() {
		nodeCount++
	}
	return &Filtergraph{Args: compilationResult.Args, Nodes: nodeCount}, nil
}

// DryRun 编译并返回完整 ffmpeg 命令行。
func DryRun(ctx context.Context, specPath string, opts Options) (*Filtergraph, error) {
	return Compile(ctx, specPath, opts)
}

// Render 编译 + 执行 ffmpeg 渲染。
func Render(ctx context.Context, specPath string, opts Options) (*Result, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	logger := opts.Logger
	if logger == nil {
		logger = slog.Default()
	}

	specification, err := spec.ParseFile(specPath)
	if err != nil {
		return nil, err
	}
	spec.ResolveAssetPaths(specification, filepath.Dir(specPath))
	if opts.Output != "" {
		specification.Output = opts.Output
	}
	if opts.GPU != "" {
		specification.HardwareAcceleration = opts.GPU
	}

	compositionTimeline, encodingOptions, finalOutputPath, err := spec.ToTimeline(specification)
	if err != nil {
		return nil, err
	}

	composerOptions := []composer.Option{composer.WithLogger(logger)}
	if encodingOptions != nil {
		composerOptions = append(composerOptions, composer.WithEncoding(*encodingOptions))
	}
	if specification.Preset != "" {
		if resolvedPreset, presetErr := resolvePlatformPreset(specification.Preset); presetErr == nil {
			composerOptions = append(composerOptions, composer.WithPreset(resolvedPreset))
		}
	}
	if specification.HardwareAcceleration != "" && strings.ToLower(specification.HardwareAcceleration) != "none" {
		accelerator := presets.HardwareAccelerator(strings.ToLower(specification.HardwareAcceleration))
		composerOptions = append(composerOptions, composer.WithHardwareAcceleration(accelerator, false))
	}

	composerInstance := composer.New(composerOptions...)

	start := time.Now()
	renderResult, err := composerInstance.Render(ctx, compositionTimeline, finalOutputPath, opts.OnProgress)
	elapsed := time.Since(start)
	if err != nil {
		return nil, err
	}

	result := &Result{Path: renderResult.OutputPath, Elapsed: elapsed, Duration: compositionTimeline.Duration()}
	if fi, statErr := os.Stat(renderResult.OutputPath); statErr == nil {
		result.Size = fi.Size()
	}
	return result, nil
}