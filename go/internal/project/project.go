// Package project 项目管理域包:项目 CRUD + 渲染触发。
// 业务代码全聚在此(对齐 HomeCast 域包架构), SQL 建表语句通过
// ProjectSchema 归属本包,由组装层收集后传入 store.Migrate。
package project

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"time"
)

// ProjectSchema 本域包的表定义。store 不解析内容,只执行传入的 schema。
var ProjectSchema = []string{
	`CREATE TABLE IF NOT EXISTS projects (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT NOT NULL,
		spec_yaml TEXT NOT NULL DEFAULT '',
		output_path TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL DEFAULT 0)`,
}

// Project 是持久化的视频编辑项目。
type Project struct {
	ID         int64
	Name       string
	SpecYAML   string
	OutputPath string
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

var (
	// ErrNotFound 表示项目不存在。
	ErrNotFound = errors.New("project not found")
	// ErrNoSpec 表示项目没有 spec 内容。
	ErrNoSpec = errors.New("project has no spec")
)

// Create 保存一个新项目,返回其 ID。
func Create(db *sql.DB, name, specYAML, outputPath string) (int64, error) {
	now := time.Now().UnixMilli()
	res, err := db.Exec(
		`INSERT INTO projects (name, spec_yaml, output_path, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?)`,
		name, specYAML, outputPath, now, now)
	if err != nil {
		return 0, fmt.Errorf("project create: %w", err)
	}
	return res.LastInsertId()
}

// Get 按 ID 读取项目。
func Get(db *sql.DB, id int64) (*Project, error) {
	row := db.QueryRow(
		`SELECT id, name, spec_yaml, output_path, created_at, updated_at
		 FROM projects WHERE id = ?`, id)
	p := &Project{}
	var created, updated int64
	err := row.Scan(&p.ID, &p.Name, &p.SpecYAML, &p.OutputPath, &created, &updated)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("project get: %w", err)
	}
	p.CreatedAt = time.UnixMilli(created)
	p.UpdatedAt = time.UnixMilli(updated)
	return p, nil
}

// List 列出全部项目(名称 + ID),按更新时间倒序。
func List(db *sql.DB) ([]Project, error) {
	rows, err := db.Query(
		`SELECT id, name, output_path, updated_at FROM projects ORDER BY updated_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("project list: %w", err)
	}
	defer rows.Close()
	var projects []Project
	for rows.Next() {
		p := Project{}
		var updated int64
		if err := rows.Scan(&p.ID, &p.Name, &p.OutputPath, &updated); err != nil {
			return nil, fmt.Errorf("project list scan: %w", err)
		}
		p.UpdatedAt = time.UnixMilli(updated)
		projects = append(projects, p)
	}
	return projects, rows.Err()
}

// Update 更新项目的 spec 与输出路径(整包替换)。
func Update(db *sql.DB, id int64, specYAML, outputPath string) error {
	res, err := db.Exec(
		`UPDATE projects SET spec_yaml = ?, output_path = ?, updated_at = ?
		 WHERE id = ?`,
		specYAML, outputPath, time.Now().UnixMilli(), id)
	if err != nil {
		return fmt.Errorf("project update: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete 删除项目。
func Delete(db *sql.DB, id int64) error {
	res, err := db.Exec(`DELETE FROM projects WHERE id = ?`, id)
	if err != nil {
		return fmt.Errorf("project delete: %w", err)
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SaveSpecToFile 把 spec 内容落盘为 YAML 文件(供 render 命令直接消费)。
func SaveSpecToFile(path, specYAML string) error {
	if err := os.MkdirAll(dirOf(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(specYAML), 0o644)
}

func dirOf(path string) string {
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == '/' || path[i] == '\\' {
			return path[:i]
		}
	}
	return "."
}
