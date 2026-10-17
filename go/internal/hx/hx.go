// Package hx htmx 端口层:每业务一个文件(对齐 HomeCast go/hx)。
// 职责:把浏览器请求翻译成 internal/project 域包调用,渲染 htmx 片段。
// 依赖:project(域包,业务唯一真相源)。
// 前端:web/templates/*.html(shell 包装 + 片段)。
// 测试:无独立测试——页面路径由 web 包模板防线 + 端到端冒烟覆盖。
// 兄弟端口:internal/api(JSON,给 AI/CLI/脚本)——同一域包,两种嘴皮。
package hx

import (
	"database/sql"
	"fmt"
	"html/template"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"vidproc/internal/cmdrender"
	"vidproc/internal/project"
	"vidproc/internal/web"
)

// H 功能上下文:只有各功能要用的数据句柄,无接口无抽象。
type H struct {
	db    *sql.DB
	tmpls *template.Template
}

// New 创建 hx 处理器。
func New(db *sql.DB) *H {
	return &H{db: db, tmpls: web.MustTemplates()}
}

// Router 组装 hx 路由(挂在 server 外层 mux 的 "/" 上)。
func (h *H) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", h.ProjectListPage)
	mux.HandleFunc("POST /project", h.ProjectCreate)
	mux.HandleFunc("GET /project/{id}", h.ProjectDetailPage)
	mux.HandleFunc("PUT /project/{id}", h.ProjectUpdate)
	mux.HandleFunc("DELETE /project/{id}", h.ProjectDelete)
	mux.HandleFunc("POST /project/{id}/render", h.ProjectRender)
	return noPageCache(mux)
}

func noPageCache(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// pageData 壳数据:媒体库(Projects) + 检查器(Project) 面板共用。
type pageData struct {
	Content  template.HTML
	Projects []project.Project
	Project  *project.Project
}

// renderPage 渲染完整页面(shell + content)。模板不支持动态嵌套名,
// 先渲染 content 注入 shell(对齐 HomeCast 同款手法)。
// shell 的媒体库/检查器面板需要 Projects + Project 数据,统一从这里透传。
func (h *H) renderPage(w http.ResponseWriter, content string, data any) {
	var sb strings.Builder
	if err := h.tmpls.ExecuteTemplate(&sb, content, data); err != nil {
		http.Error(w, "render: "+err.Error(), 500)
		return
	}
	projects, err := project.List(h.db)
	if err != nil {
		http.Error(w, "list projects: "+err.Error(), 500)
		return
	}
	err = h.tmpls.ExecuteTemplate(w, "shell.html", pageData{
		Content:  template.HTML(sb.String()),
		Projects: projects,
	})
	if err != nil {
		http.Error(w, "shell: "+err.Error(), 500)
	}
}

// renderPageWithProject 同 renderPage,但检查器面板显示指定项目。
func (h *H) renderPageWithProject(w http.ResponseWriter, content string, p *project.Project) {
	data := struct{ Project *project.Project }{Project: p}
	var sb strings.Builder
	if err := h.tmpls.ExecuteTemplate(&sb, content, data); err != nil {
		http.Error(w, "render: "+err.Error(), 500)
		return
	}
	projects, err := project.List(h.db)
	if err != nil {
		http.Error(w, "list projects: "+err.Error(), 500)
		return
	}
	err = h.tmpls.ExecuteTemplate(w, "shell.html", pageData{
		Content:  template.HTML(sb.String()),
		Projects: projects,
		Project:  p,
	})
	if err != nil {
		http.Error(w, "shell: "+err.Error(), 500)
	}
}

// ProjectListPage GET / → 项目列表页。
func (h *H) ProjectListPage(w http.ResponseWriter, r *http.Request) {
	projects, err := project.List(h.db)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.renderPage(w, "project_list.html", struct{ Projects []project.Project }{Projects: projects})
}

// ProjectCreate POST /project → 新建项目,返回刷新后的列表片段。
func (h *H) ProjectCreate(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	name := strings.TrimSpace(r.FormValue("name"))
	spec := r.FormValue("spec")
	if name == "" || strings.TrimSpace(spec) == "" {
		http.Error(w, "name and spec required", 400)
		return
	}
	if _, err := project.Create(h.db, name, spec, r.FormValue("output")); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	projects, err := project.List(h.db)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.tmpls.ExecuteTemplate(w, "project_list.html", struct{ Projects []project.Project }{Projects: projects})
}

// ProjectDetailPage GET /project/{id} → 项目详情页。
func (h *H) ProjectDetailPage(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	p, err := project.Get(h.db, id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	h.renderPageWithProject(w, "project_detail.html", p)
}

// ProjectUpdate PUT /project/{id} → 更新项目,返回刷新后的详情片段。
func (h *H) ProjectUpdate(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	p, err := project.Get(h.db, id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}
	spec := r.FormValue("spec")
	if strings.TrimSpace(spec) == "" {
		spec = p.SpecYAML
	}
	out := r.FormValue("output")
	if out == "" {
		out = p.OutputPath
	}
	if err := project.Update(h.db, id, spec, out); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	// 名称也在 projects 表里;Update 不碰 name,单独处理
	if name := strings.TrimSpace(r.FormValue("name")); name != "" && name != p.Name {
		if _, err := h.db.Exec(`UPDATE projects SET name = ?, updated_at = ? WHERE id = ?`, name, time.Now().UnixMilli(), id); err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
	}
	updated, err := project.Get(h.db, id)
	if err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	h.tmpls.ExecuteTemplate(w, "project_detail.html", struct{ Project *project.Project }{Project: updated})
}

// ProjectDelete DELETE /project/{id} → 删除,重定向列表。
func (h *H) ProjectDelete(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	if err := project.Delete(h.db, id); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	w.Header().Set("HX-Redirect", "/")
	w.WriteHeader(204)
}

// ProjectRender POST /project/{id}/render → 从 SQLite 取 spec,走引擎渲染,返回结果片段。
func (h *H) ProjectRender(w http.ResponseWriter, r *http.Request) {
	id, err := pathID(r)
	if err != nil {
		http.Error(w, err.Error(), 400)
		return
	}
	p, err := project.Get(h.db, id)
	if err != nil {
		http.Error(w, err.Error(), 404)
		return
	}

	tmpSpec := filepath.Join(os.TempDir(), fmt.Sprintf("vidproc_web_%d.yaml", id))
	if err := project.SaveSpecToFile(tmpSpec, p.SpecYAML); err != nil {
		http.Error(w, err.Error(), 500)
		return
	}
	defer os.Remove(tmpSpec)

	data := map[string]string{}
	start := time.Now()
	out, err := cmdrender.Render(r.Context(), tmpSpec, cmdrender.Options{Output: p.OutputPath})
	elapsed := time.Since(start).Round(10 * time.Millisecond).String()
	if err != nil {
		data["Error"] = err.Error()
		data["Elapsed"] = elapsed
	} else {
		sizeMB := float64(out.Size) / (1 << 20)
		data["Output"] = out.Path
		data["SizeMB"] = fmt.Sprintf("%.2f", sizeMB)
		data["Duration"] = out.Duration.String()
		data["Elapsed"] = elapsed
	}
	h.tmpls.ExecuteTemplate(w, "render_result.html", data)
}

func pathID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}