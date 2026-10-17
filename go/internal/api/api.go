// Package api JSON 端口层:每业务一个文件(对齐 HomeCast internal/api)。
//
// 与 hx(htmx 端口)共享 internal/project 域包——业务逻辑只写一份,
// 两种嘴皮: JSON(给 AI/CLI/脚本) + htmx 片段(给浏览器)。
package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"vidproc/internal/project"
)

// ProjectJSON 项目 JSON 表示(不含大段 spec,列表场景省带宽)。
type ProjectJSON struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	OutputPath string `json:"output_path,omitempty"`
	CreatedAt  string `json:"created_at,omitempty"`
	UpdatedAt  string `json:"updated_at,omitempty"`
}

// projectDetail spec 完整带出(get 用)。
type projectDetail struct {
	ProjectJSON
	SpecYAML string `json:"spec_yaml"`
}

func toJSON(p *project.Project) ProjectJSON {
	return ProjectJSON{
		ID:         p.ID,
		Name:       p.Name,
		OutputPath: p.OutputPath,
		CreatedAt:  p.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:  p.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// Router 挂载 JSON API 路由(前缀 /api)。
func Router(db *sql.DB) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/projects", func(w http.ResponseWriter, r *http.Request) {
		projects, err := project.List(db)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		out := make([]ProjectJSON, 0, len(projects))
		for _, p := range projects {
			out = append(out, toJSON(&p))
		}
		writeJSON(w, 200, out)
	})

	mux.HandleFunc("POST /api/projects", func(w http.ResponseWriter, r *http.Request) {
		var req projectDetail
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, 400, "bad json: "+err.Error())
			return
		}
		if req.Name == "" || req.SpecYAML == "" {
			writeErr(w, 400, "name and spec_yaml required")
			return
		}
		id, err := project.Create(db, req.Name, req.SpecYAML, req.OutputPath)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		p, err := project.Get(db, id)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 201, toJSON(p))
	})

	mux.HandleFunc("GET /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		writeJSON(w, 200, projectDetail{ProjectJSON: toJSON(p), SpecYAML: p.SpecYAML})
	})

	mux.HandleFunc("PUT /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		var req struct {
			Name       *string `json:"name"`
			SpecYAML   *string `json:"spec_yaml"`
			OutputPath *string `json:"output_path"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeErr(w, 400, "bad json: "+err.Error())
			return
		}
		newSpec, newOut := p.SpecYAML, p.OutputPath
		if req.SpecYAML != nil {
			newSpec = *req.SpecYAML
		}
		if req.OutputPath != nil {
			newOut = *req.OutputPath
		}
		if err := project.Update(db, p.ID, newSpec, newOut); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		// 名称单独更新(Update 不碰 name,对齐 hx 层行为)
		if req.Name != nil && *req.Name != "" && *req.Name != p.Name {
			if _, err := db.Exec(`UPDATE projects SET name = ? WHERE id = ?`, *req.Name, p.ID); err != nil {
				writeErr(w, 500, err.Error())
				return
			}
		}
		updated, err := project.Get(db, p.ID)
		if err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		writeJSON(w, 200, projectDetail{ProjectJSON: toJSON(updated), SpecYAML: updated.SpecYAML})
	})

	mux.HandleFunc("DELETE /api/projects/{id}", func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		if err := project.Delete(db, p.ID); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		w.WriteHeader(204)
	})

	return mux
}

// getProject 解析路径 id 并取项目,失败时已写错误响应。
func getProject(w http.ResponseWriter, r *http.Request, db *sql.DB) (*project.Project, error) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad id")
		return nil, err
	}
	p, err := project.Get(db, id)
	if err != nil {
		if err == project.ErrNotFound {
			writeErr(w, 404, "project not found")
		} else {
			writeErr(w, 500, err.Error())
		}
		return nil, err
	}
	return p, nil
}