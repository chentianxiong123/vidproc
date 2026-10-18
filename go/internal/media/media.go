// Package media 媒体资产域包:项目素材库(上传落盘 + ffprobe 探测元数据)。
// 对齐 HomeCast 域包架构:SQL 建表归本包(MediaSchema),store 只执行;
// 业务(探测/入库/删除)全聚在此,api 层只做嘴皮。
package media

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MediaSchema 素材表定义(store 不解析内容,只执行)。
var MediaSchema = []string{
	`CREATE TABLE IF NOT EXISTS media_assets (
		id          INTEGER PRIMARY KEY AUTOINCREMENT,
		project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
		file_name   TEXT NOT NULL,
		file_path   TEXT NOT NULL,
		file_type   TEXT NOT NULL DEFAULT 'video', -- video | audio | image
		file_size   INTEGER NOT NULL DEFAULT 0,
		duration_s  REAL NOT NULL DEFAULT 0,
		width       INTEGER NOT NULL DEFAULT 0,
		height      INTEGER NOT NULL DEFAULT 0,
		frame_rate  REAL NOT NULL DEFAULT 0,
		sample_rate INTEGER NOT NULL DEFAULT 0,
		channels    INTEGER NOT NULL DEFAULT 0,
		created_at  INTEGER NOT NULL
	)`,
	`CREATE INDEX IF NOT EXISTS idx_media_project ON media_assets(project_id)`,
}

// Asset 媒体素材(上传文件的元数据 + 磁盘路径)。
type Asset struct {
	ID         int64
	ProjectID  int64
	FileName   string
	FilePath   string // 绝对路径(引擎 spec 直接引用)
	FileType   string // video | audio | image
	FileSize   int64
	DurationS  float64
	Width      int
	Height     int
	FrameRate  float64
	SampleRate int
	Channels   int
	CreatedAt  time.Time
}

var (
	// ErrNotFound 素材不存在。
	ErrNotFound = fmt.Errorf("media asset not found")
)

// ProbeResult 探测结果(由 ffprobe 填)。
type ProbeResult struct {
	DurationS  float64
	Width      int
	Height     int
	FrameRate  float64
	SampleRate int
	Channels   int
	HasVideo   bool
	HasAudio   bool
}

// Create 保存一个新素材记录。FilePath 统一正斜杠(Windows 反斜杠在
// YAML 双引号里是转义符,spec 里出现 \U 会被 YAML 解析器拒绝;
// ffmpeg 双平台接受正斜杠)。规范:库内路径一律正斜杠。
func Create(db *sql.DB, a *Asset) (int64, error) {
	a.FilePath = filepath.ToSlash(filepath.Clean(a.FilePath))
	res, err := db.Exec(
		`INSERT INTO media_assets (project_id, file_name, file_path, file_type, file_size,
			duration_s, width, height, frame_rate, sample_rate, channels, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		a.ProjectID, a.FileName, a.FilePath, a.FileType, a.FileSize,
		a.DurationS, a.Width, a.Height, a.FrameRate, a.SampleRate, a.Channels,
		a.CreatedAt.UnixMilli(),
	)
	if err != nil {
		return 0, fmt.Errorf("media.Create: %w", err)
	}
	return res.LastInsertId()
}

// List 列出项目的全部素材(新 → 旧)。
func List(db *sql.DB, projectID int64) ([]Asset, error) {
	rows, err := db.Query(
		`SELECT id, project_id, file_name, file_path, file_type, file_size,
			duration_s, width, height, frame_rate, sample_rate, channels, created_at
		 FROM media_assets WHERE project_id = ? ORDER BY created_at DESC, id DESC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("media.List: %w", err)
	}
	defer rows.Close()

	var out []Asset
	for rows.Next() {
		var a Asset
		var ms int64
		if err := rows.Scan(&a.ID, &a.ProjectID, &a.FileName, &a.FilePath, &a.FileType,
			&a.FileSize, &a.DurationS, &a.Width, &a.Height, &a.FrameRate,
			&a.SampleRate, &a.Channels, &ms); err != nil {
			return nil, fmt.Errorf("media.List scan: %w", err)
		}
		a.CreatedAt = time.UnixMilli(ms)
		a.FilePath = filepath.ToSlash(a.FilePath)
		out = append(out, a)
	}
	return out, rows.Err()
}

// Get 按 ID 取素材。
func Get(db *sql.DB, id int64) (*Asset, error) {
	row := db.QueryRow(
		`SELECT id, project_id, file_name, file_path, file_type, file_size,
			duration_s, width, height, frame_rate, sample_rate, channels, created_at
		 FROM media_assets WHERE id = ?`, id)
	var a Asset
	var ms int64
	if err := row.Scan(&a.ID, &a.ProjectID, &a.FileName, &a.FilePath, &a.FileType,
		&a.FileSize, &a.DurationS, &a.Width, &a.Height, &a.FrameRate,
		&a.SampleRate, &a.Channels, &ms); err != nil {
		if err == sql.ErrNoRows {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("media.Get: %w", err)
	}
	a.CreatedAt = time.UnixMilli(ms)
	a.FilePath = filepath.ToSlash(a.FilePath)
	return &a, nil
}

// Delete 删除素材记录并移除磁盘文件。磁盘文件缺失不报错(可重复删)。
func Delete(db *sql.DB, id int64) error {
	a, err := Get(db, id)
	if err != nil {
		return err
	}
	if _, err := db.Exec(`DELETE FROM media_assets WHERE id = ?`, id); err != nil {
		return fmt.Errorf("media.Delete: %w", err)
	}
	if a.FilePath != "" {
		_ = os.Remove(a.FilePath)
	}
	return nil
}

// FileTypeFromName 按扩展名推断素材类型(video/audio/image)。
func FileTypeFromName(name string) string {
	ext := strings.ToLower(strings.TrimPrefix(filepath.Ext(name), "."))
	switch ext {
	case "mp3", "wav", "aac", "flac", "ogg", "m4a", "wma":
		return "audio"
	case "png", "jpg", "jpeg", "webp", "bmp", "gif", "svg":
		return "image"
	default: // mp4/mov/mkv/webm/avi 等
		return "video"
	}
}
