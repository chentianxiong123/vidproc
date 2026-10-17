# wails3 桌面壳

vidproc 的系统壳:Wails3 只做窗口,业务走真实 HTTP(对齐 HomeCast 迁移模式)。

## 结构

```
wails3/
├── main.go            ← 壳入口:起后端 + Wails3 窗口(URL 直指 http://127.0.0.1:8398)
├── http_desktop.go    ← 桌面后端监听(127.0.0.1)
├── ensurehome_other.go← 数据目录(~/.config/vidproc)
├── frontend/          ← 空壳(无 JS bindings,main.ts 一行注释)
└── go.mod             ← module vidproc/app; replace vidproc => ../go
```

## 构建

```bash
cd wails3
go build -o vidproc-studio.exe .
```

产出独立桌面 exe(含 go/ 全部引擎 + SQLite + htmx 页面,go:embed 内嵌)。

## 三形态同一核心

| 形态 | 命令 | 产物 |
|------|------|------|
| CLI | `go build ./cmd/vidproc` | vidproc.exe |
| Web | `vidproc studio` | htmx Web Studio :8398 |
| 桌面 | `wails3/go build` | vidproc-studio.exe |

业务代码唯一:`go/internal/**` 垂直切片,三种形态共用。wails 壳零业务。
