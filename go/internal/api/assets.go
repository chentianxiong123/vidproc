// assets.go — 媒体资产 API:/api/projects/{id}/assets(GET 列表 / POST 上传)+ DELETE。
// 上传:multipart 收文件 → 落盘到项目 media 目录 → ffprobe 探测 → media 域包入库。
package api

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vidproc/internal/ffmpeg"
	"vidproc/internal/media"
	"vidproc/internal/project"
)

// AssetJSON 素材 JSON(前端媒体库渲染)。
type AssetJSON struct {
	ID         int64   `json:"id"`
	ProjectID  int64   `json:"project_id"`
	FileName   string  `json:"file_name"`
	FilePath   string  `json:"file_path"`
	FileType   string  `json:"file_type"` // video | audio | image
	FileSize   int64   `json:"file_size"`
	DurationS  float64 `json:"duration_seconds"`
	Width      int     `json:"width"`
	Height     int     `json:"height"`
	FrameRate  float64 `json:"frame_rate"`
	ThumbURL   string  `json:"thumbnail_url,omitempty"` // 视频首帧 / 图片原图
	SampleRate int     `json:"sample_rate"`
	Channels   int     `json:"channels"`
	CreatedAt  string  `json:"created_at"`
}

func assetToJSON(a *media.Asset) AssetJSON {
	aj := AssetJSON{
		ID:         a.ID,
		ProjectID:  a.ProjectID,
		FileName:   a.FileName,
		FilePath:   a.FilePath,
		FileType:   a.FileType,
		FileSize:   a.FileSize,
		DurationS:  a.DurationS,
		Width:      a.Width,
		Height:     a.Height,
		FrameRate:  a.FrameRate,
		SampleRate: a.SampleRate,
		Channels:   a.Channels,
		CreatedAt:  a.CreatedAt.Format("2006-01-02 15:04:05"),
	}
	// 缩略图地址(视频/图片走 /frame 端点抽帧,音频无缩略图)
	thumb := mediaThumbURL(a)
	if thumb != "" {
		aj.ThumbURL = thumb
	}
	return aj
}

// mediaBaseDir 素材落盘根目录(与服务数据目录同级;server.New 传入)。
var mediaBaseDir = ""

// SetMediaDir 由 server 组装层设置素材根目录。
func SetMediaDir(dir string) { mediaBaseDir = dir }

// mediaThumbURL 素材缩略图 URL:视频 → 该 asset 抽首帧;图片 → 直接 /assets/frame。
func mediaThumbURL(a *media.Asset) string {
	switch a.FileType {
	case "video":
		return fmt.Sprintf("/api/assets/%d/frame?t=0", a.ID)
	case "image":
		return fmt.Sprintf("/api/assets/%d/frame?t=0", a.ID)
	default:
		return ""
	}
}

// probeWeb 用 ffprobe 探测文件,填 media.ProbeResult。
func probeWeb(path string) (media.ProbeResult, error) {
	cfg, err := ffmpeg.NewConfig()
	if err != nil {
		return media.ProbeResult{}, fmt.Errorf("ffprobe 不可用: %w", err)
	}
	pr, err := cfg.Probe(path)
	if err != nil {
		return media.ProbeResult{}, err
	}
	var out media.ProbeResult
	for _, s := range pr.Streams {
		if s.CodecType == "video" {
			out.HasVideo = true
			out.Width = s.Width
			out.Height = s.Height
			if parts := strings.SplitN(s.RFrameRate, "/", 2); len(parts) == 2 {
				if num, e1 := strconv.ParseFloat(parts[0], 64); e1 == nil {
					if den, e2 := strconv.ParseFloat(parts[1], 64); e2 == nil && den != 0 {
						out.FrameRate = num / den
					}
				}
			}
		} else if s.CodecType == "audio" {
			out.HasAudio = true
			if n, err := strconv.Atoi(s.SampleRate); err == nil {
				out.SampleRate = n
			}
			out.Channels = s.Channels
		}
	}
	// duration:format 里取
	if d := pr.FormatDuration(); d > 0 {
		out.DurationS = d
	}
	sampleRate := out.SampleRate
	_ = sampleRate
	return out, nil
}

// AssetsListHandler GET /api/projects/{id}/assets → 项目素材列表。
func AssetsListHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		assets, err := media.List(db, p.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out := make([]AssetJSON, 0, len(assets))
		for i := range assets {
			out = append(out, assetToJSON(&assets[i]))
		}
		writeJSON(w, 200, out)
	}
}

// AssetsUploadHandler POST /api/projects/{id}/assets → multipart 上传素材。
func AssetsUploadHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		// 30MB 上限(视频素材常见尺寸;可调)
		if err := r.ParseMultipartForm(30 << 20); err != nil {
			writeErr(w, 400, "multipart 解析失败: "+err.Error())
			return
		}
		file, header, err := r.FormFile("file")
		if err != nil {
			writeErr(w, 400, "缺少 file 字段")
			return
		}
		defer file.Close()

		// 校验扩展名
		fileName := filepath.Base(header.Filename)
		ftype := media.FileTypeFromName(fileName)
		if ftype == "" {
			writeErr(w, 400, "不支持的素材类型: "+fileName)
			return
		}

		// 落盘:项目专属目录,避免同名冲突加时间戳
		dir := filepath.Join(mediaBaseDir, "projects", strconv.FormatInt(p.ID, 10), "media")
		if err := os.MkdirAll(dir, 0o755); err != nil {
			writeErr(w, 500, "创建素材目录失败: "+err.Error())
			return
		}
		storedName := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), fileName)
		dst := filepath.Join(dir, storedName)
		out, err := os.Create(dst)
		if err != nil {
			writeErr(w, 500, "写素材失败: "+err.Error())
			return
		}
		size, err := io.Copy(out, file)
		cerr := out.Close()
		if err != nil || cerr != nil {
			os.Remove(dst)
			writeErr(w, 500, "写素材失败")
			return
		}

		// ffprobe 探测(视频/音频;图片跳过)
		pr, _ := probeWeb(dst) // 探测失败不阻断上传(纯元数据)
		id, err := media.Create(db, &media.Asset{
			ProjectID:  p.ID,
			FileName:   fileName,
			FilePath:   dst,
			FileType:   ftype,
			FileSize:   size,
			DurationS:  pr.DurationS,
			Width:      pr.Width,
			Height:     pr.Height,
			FrameRate:  pr.FrameRate,
			SampleRate: pr.SampleRate,
			Channels:   pr.Channels,
			CreatedAt:  time.Now(),
		})
		if err != nil {
			os.Remove(dst)
			writeErr(w, 500, err.Error())
			return
		}
		a, _ := media.Get(db, id)
		writeJSON(w, 201, assetToJSON(a))
	}
}

// ImportMediaFile 桌面导入:复制本地文件到项目媒体目录 + ffprobe 探测 + 资产入库。
// 供 wails3 桌面壳 ImportLocalMedia 绑定复用(与 HTTP 上传同一落盘/入库规则)。
func ImportMediaFile(db *sql.DB, projectID int64, srcPath string) (*media.Asset, error) {
	fileName := filepath.Base(srcPath)
	ftype := media.FileTypeFromName(fileName)
	if ftype == "" {
		return nil, errors.New("不支持的素材类型: " + fileName)
	}
	dir := filepath.Join(mediaBaseDir, "projects", strconv.FormatInt(projectID, 10), "media")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	storedName := fmt.Sprintf("%d_%s", time.Now().UnixMilli(), fileName)
	dst := filepath.Join(dir, storedName)
	in, err := os.Open(srcPath)
	if err != nil {
		return nil, err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return nil, err
	}
	size, err := io.Copy(out, in)
	cerr := out.Close()
	if err != nil || cerr != nil {
		os.Remove(dst)
		return nil, err
	}
	pr, _ := probeWeb(dst) // 探测失败不阻断导入(纯元数据)
	id, err := media.Create(db, &media.Asset{
		ProjectID:  projectID,
		FileName:   fileName,
		FilePath:   dst,
		FileType:   ftype,
		FileSize:   size,
		DurationS:  pr.DurationS,
		Width:      pr.Width,
		Height:     pr.Height,
		FrameRate:  pr.FrameRate,
		SampleRate: pr.SampleRate,
		Channels:   pr.Channels,
		CreatedAt:  time.Now(),
	})
	if err != nil {
		os.Remove(dst)
		return nil, err
	}
	return media.Get(db, id)
}

// AssetsDeleteHandler DELETE /api/assets/{id} → 删素材(记录 + 磁盘文件)。
func AssetsDeleteHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
		if err != nil {
			writeErr(w, 400, "bad id")
			return
		}
		if err := media.Delete(db, id); err != nil {
			if err == media.ErrNotFound {
				writeErr(w, 404, "asset not found")
			} else {
				writeErr(w, 500, err.Error())
			}
			return
		}
		w.WriteHeader(204)
	}
}

// ThumbFrameHandler GET /api/assets/{id}/frame?t=0 → 素材缩略图(视频首帧/图片原图)。
// 复用 preview.go 的 extractFrame(同一 ffmpeg 通道)。
func ThumbFrameHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		a, err := media.Get(db, mustID(r.PathValue("id")))
		if err != nil {
			if err == media.ErrNotFound {
				writeErr(w, 404, "asset not found")
			} else {
				writeErr(w, 500, err.Error())
			}
			return
		}
		t := 0.0
		if raw := r.URL.Query().Get("t"); raw != "" {
			if f, perr := strconv.ParseFloat(raw, 64); perr == nil && f >= 0 {
				t = f
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		// 图片素材:直接回原图(不抽帧)
		if a.FileType == "image" {
			http.ServeFile(w, r, a.FilePath)
			return
		}
		jpeg, err := extractFrame(ctx, a.FilePath, t, FrameOptions{Width: 240})
		if err != nil {
			writeErr(w, 502, err.Error())
			return
		}
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=3600")
		w.Write(jpeg)
	}
}

var _ = project.Project{} // 保 import(后续 asset 端点需要项目域)
