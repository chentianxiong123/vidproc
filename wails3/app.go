package main

// 桌面壳绑定(wails v3):JS 侧 window.go.main.App.<Method>()
// 对齐参考 useDesktop:SelectMediaFiles / SaveProjectFileDialog / OpenProjectFileDialog /
// SelectExportDirectory / SaveVideoFileDialog / GetSystemCapabilities / GetServerInfo / ImportLocalMedia。
// 纯 Web 模式(vidproc.exe studio)没有这些绑定 → 前端 isDesktop 分支自动回退浏览器控件。

import (
	"database/sql"
	"os/exec"
	"runtime"
	"strings"

	"github.com/wailsapp/wails/v3/pkg/application"

	"vidproc/internal/api"
	"vidproc/internal/ffmpeg"
)

// App 桌面壳方法接收器。
type App struct {
	db *sql.DB
}

func newApp(db *sql.DB) *App {
	return &App{db: db}
}

// SelectMediaFiles 原生多选媒体文件对话框(对齐 go.desktop.App.SelectMediaFiles)。
func (a *App) SelectMediaFiles() ([]string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle("选择媒体文件").
		AddFilter("媒体文件", "*.mp4;*.mov;*.webm;*.mkv;*.m4v;*.mp3;*.wav;*.aac;*.flac;*.ogg;*.png;*.jpg;*.jpeg;*.webp;*.gif;*.srt;*.vtt;*.ass").
		PromptForMultipleSelection()
}

// SaveProjectFileDialog 保存项目对话框(默认 project.vidonex.json)。
func (a *App) SaveProjectFileDialog(defaultName string) (string, error) {
	if defaultName == "" {
		defaultName = "project.vidonex.json"
	}
	return application.Get().Dialog.SaveFile().
		SetMessage("保存项目").
		AddFilter("Vidonex 项目", "*.json").
		PromptForSingleSelection()
}

// OpenProjectFileDialog 打开项目对话框。
func (a *App) OpenProjectFileDialog() (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle("打开项目").
		AddFilter("Vidonex 项目", "*.json").
		PromptForSingleSelection()
}

// SelectExportDirectory 选择导出目录(目录选择对话框)。
func (a *App) SelectExportDirectory() (string, error) {
	return application.Get().Dialog.OpenFile().
		SetTitle("选择导出目录").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		CanCreateDirectories(true).
		PromptForSingleSelection()
}

// SaveVideoFileDialog 保存渲染视频对话框(默认名 + 格式扩展)。
func (a *App) SaveVideoFileDialog(defaultName string, format string) (string, error) {
	if defaultName == "" {
		defaultName = "rendered_composition.mp4"
	}
	ext := "." + format
	if len(format) > 0 && format[0] != '.' {
		ext = "." + format
	}
	return application.Get().Dialog.SaveFile().
		SetMessage("保存渲染视频").
		AddFilter("视频文件", "*"+ext).
		PromptForSingleSelection()
}

// GetSystemCapabilities 系统能力探测(供渲染 GPU 选项与 About 诊断)。
func (a *App) GetSystemCapabilities() map[string]any {
	ffVer := "不可用"
	if ff, err := ffmpeg.NewConfig(); err == nil {
		ffVer = ff.Version()
	}
	return map[string]any{
		"os":                     runtime.GOOS,
		"arch":                   runtime.GOARCH,
		"cpu_count":              runtime.NumCPU(),
		"available_accelerators": ffmpegAccelerators(),
		"ffmpeg_version":         ffVer,
		"go_version":             runtime.Version(),
	}
}

// ffmpegAccelerators 扫描 ffmpeg -encoders 探测硬件加速器(nvenc/cuda/vaapi/qsv)。
func ffmpegAccelerators() []string {
	out := []string{}
	ff, err := ffmpeg.NewConfig()
	if err != nil {
		return out
	}
	cmd := exec.Command(ff.FFmpeg, "-hide_banner", "-encoders")
	b, err := cmd.Output()
	if err != nil {
		return out
	}
	lower := strings.ToLower(string(b))
	for _, acc := range []string{"nvenc", "cuda", "vaapi", "qsv"} {
		if strings.Contains(lower, acc) {
			out = append(out, acc)
		}
	}
	return out
}

// GetServerInfo 后端连接信息(端口/URL;参考 useDesktop cachedServerInfo)。
func (a *App) GetServerInfo() map[string]string {
	return map[string]string{
		"port": backendPort(),
		"url":  "http://127.0.0.1:" + backendPort(),
		"ws":   "ws://127.0.0.1:" + backendPort() + "/ws",
	}
}

// ImportLocalMedia 批量导入本地文件到项目(对齐 go.desktop.App.ImportLocalMedia)。
func (a *App) ImportLocalMedia(projectID int64, filePaths []string) ([]map[string]any, error) {
	out := []map[string]any{}
	for _, fp := range filePaths {
		asset, err := api.ImportMediaFile(a.db, projectID, fp)
		if err != nil {
			continue // 单个失败不阻断批量
		}
		out = append(out, map[string]any{
			"id":               asset.ID,
			"file_name":        asset.FileName,
			"file_path":        asset.FilePath,
			"file_type":        asset.FileType,
			"file_size":        asset.FileSize,
			"duration_seconds": asset.DurationS,
		})
	}
	return out, nil
}