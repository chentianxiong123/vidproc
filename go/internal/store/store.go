// Package store 本地持久层:SQLite 单文件(ncruces/go-sqlite3,纯 Go 无 CGO,
// Android/WASM 可交叉编译)。
//
// 对齐 HomeCast 2026-10-17 域包架构:store 只负责打开/执行传入的 schema,
// **不知道任何业务表**。业务表定义在各自域包(XxxSchema 字符串),
// 由组装层(server.go / cmd)收集后传入 Migrate —— 固定依赖方向:
// 组装层 → service 域包 + store,store 永不 import 业务包(踩坑 #1 解法)。
package store

import (
	"database/sql"
	"os"
	"path/filepath"

	_ "github.com/ncruces/go-sqlite3/driver"
)

// Open 打开(不存在则创建)SQLite 文件。
func Open(path string) (*sql.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite3", path)
	if err != nil {
		return nil, err
	}
	return db, nil
}

// Migrate 执行传入的 schema 列表(来自各业务域包的 XxxSchema)。
// schemas:业务建表语句;store 不解析内容,只逐条执行。
func Migrate(db *sql.DB, schemas []string) error {
	for _, s := range schemas {
		if _, err := db.Exec(s); err != nil {
			return err
		}
	}
	return nil
}
