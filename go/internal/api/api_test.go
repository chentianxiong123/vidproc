package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"vidproc/internal/project"
	"vidproc/internal/store"
)

func newTestRouter(t *testing.T) http.Handler {
	t.Helper()
	db, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := store.Migrate(db, project.ProjectSchema); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return Router(db)
}

func doJSON(t *testing.T, h http.Handler, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func TestProjectsCRUDJSON(t *testing.T) {
	h := newTestRouter(t)

	// 创建
	rr := doJSON(t, h, "POST", "/api/projects",
		`{"name":"api_test","spec_yaml":"canvas: 1920x1080","output_path":"out.mp4"}`)
	if rr.Code != 201 {
		t.Fatalf("create = %d, want 201 (%s)", rr.Code, rr.Body.String())
	}
	var created ProjectJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if created.ID <= 0 || created.Name != "api_test" {
		t.Fatalf("created = %+v", created)
	}

	// 列表
	rr = doJSON(t, h, "GET", "/api/projects", "")
	if rr.Code != 200 {
		t.Fatalf("list = %d, want 200", rr.Code)
	}
	var list []ProjectJSON
	if err := json.Unmarshal(rr.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(list) != 1 || list[0].Name != "api_test" {
		t.Fatalf("list = %+v", list)
	}

	// 详情(spec 完整带出)
	rr = doJSON(t, h, "GET", "/api/projects/1", "")
	if rr.Code != 200 {
		t.Fatalf("get = %d, want 200", rr.Code)
	}
	var detail projectDetail
	if err := json.Unmarshal(rr.Body.Bytes(), &detail); err != nil {
		t.Fatalf("decode detail: %v", err)
	}
	if detail.SpecYAML != "canvas: 1920x1080" {
		t.Fatalf("detail spec = %q", detail.SpecYAML)
	}

	// 更新
	rr = doJSON(t, h, "PUT", "/api/projects/1",
		`{"spec_yaml":"canvas: 1280x720","name":"renamed"}`)
	if rr.Code != 200 {
		t.Fatalf("update = %d, want 200 (%s)", rr.Code, rr.Body.String())
	}
	rr = doJSON(t, h, "GET", "/api/projects/1", "")
	var updated projectDetail
	json.Unmarshal(rr.Body.Bytes(), &updated)
	if updated.SpecYAML != "canvas: 1280x720" || updated.Name != "renamed" {
		t.Fatalf("update not applied: %+v", updated)
	}

	// 删除
	rr = doJSON(t, h, "DELETE", "/api/projects/1", "")
	if rr.Code != 204 {
		t.Fatalf("delete = %d, want 204", rr.Code)
	}
	rr = doJSON(t, h, "GET", "/api/projects/1", "")
	if rr.Code != 404 {
		t.Fatalf("get after delete = %d, want 404", rr.Code)
	}
}

func TestProjectsValidation(t *testing.T) {
	h := newTestRouter(t)

	// 缺 name
	rr := doJSON(t, h, "POST", "/api/projects", `{"spec_yaml":"x"}`)
	if rr.Code != 400 {
		t.Fatalf("missing name = %d, want 400", rr.Code)
	}
	// 缺 spec
	rr = doJSON(t, h, "POST", "/api/projects", `{"name":"x"}`)
	if rr.Code != 400 {
		t.Fatalf("missing spec = %d, want 400", rr.Code)
	}
	// 坏 JSON
	rr = doJSON(t, h, "POST", "/api/projects", `{not json`)
	if rr.Code != 400 {
		t.Fatalf("bad json = %d, want 400", rr.Code)
	}
	// 不存在 id
	rr = doJSON(t, h, "GET", "/api/projects/999", "")
	if rr.Code != 404 {
		t.Fatalf("missing id = %d, want 404", rr.Code)
	}
}
// TestSpecGraphMermaid:合法 spec → graph API 输出 mermaid 文本(对齐 GraphVisualizerModal 数据源)。
func TestSpecGraphMermaid(t *testing.T) {
	h := newTestRouter(t)
	// 用 JSON 编码 spec,避免手写转义
	spec := `version: "1.0"
canvas: {width: 1280, height: 720, background_color: "black"}
fps: "30"
tracks:
  - id: "v1"
    kind: "video"
    clips:
      - {id: "c1", source: "in.mp4", duration: "2s"}
`
	body, _ := json.Marshal(map[string]string{"name": "graph_test", "spec_yaml": spec})
	rr := doJSON(t, h, "POST", "/api/projects", string(body))
	if rr.Code != 201 && rr.Code != 200 {
		t.Fatalf("create = %d, want 2xx", rr.Code)
	}
	var created struct{ ID int64 `json:"id"` }
	if err := json.Unmarshal(rr.Body.Bytes(), &created); err != nil || created.ID == 0 {
		t.Fatalf("no id in create response: %v", rr.Body.String())
	}
	rr = doJSON(t, h, "GET", "/api/projects/1/spec/graph", "")
	if rr.Code != 200 {
		t.Fatalf("graph = %d, want 200 (body: %s)", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "graph LR") {
		t.Fatalf("mermaid missing graph LR: %.200s", rr.Body.String())
	}
	// dot 格式
	rr = doJSON(t, h, "GET", "/api/projects/1/spec/graph?format=dot", "")
	if rr.Code != 200 {
		t.Fatalf("graph dot = %d, want 200", rr.Code)
	}
	if !strings.Contains(rr.Body.String(), "digraph") {
		t.Fatalf("dot missing digraph: %.200s", rr.Body.String())
	}
	// 不存在项目
	rr = doJSON(t, h, "GET", "/api/projects/999/spec/graph", "")
	if rr.Code != 404 {
		t.Fatalf("missing project graph = %d, want 404", rr.Code)
	}
}
