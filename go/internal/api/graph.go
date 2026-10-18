package api

// SpecGraphHandler GET /api/projects/{id}/spec/graph?format=mermaid|dot
// 返回项目 spec 编译出的 filtergraph DAG 可视化文本(GraphVisualizerModal 数据源)。
// 构建链路:ParseYAML → spec.ToTimeline → compiler.Compile → Mermaid()/DOT()。
import (
	"database/sql"
	"log/slog"
	"net/http"
	"strings"

	"vidproc/internal/render/compiler"
	"vidproc/internal/timeline/spec"
)

func SpecGraphHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		vs, err := spec.ParseYAML(strings.NewReader(p.SpecYAML))
		if err != nil {
			writeErr(w, 400, "parse spec: "+err.Error())
			return
		}
		compositionTimeline, encodingOptions, outputPath, err := spec.ToTimeline(vs)
		if err != nil {
			writeErr(w, 400, "spec to timeline: "+err.Error())
			return
		}
		compilerInstance := compiler.New(slog.Default())
		compilerInstance.SetEncodingOptions(*encodingOptions)
		result, err := compilerInstance.Compile(compositionTimeline, outputPath)
		if err != nil {
			writeErr(w, 400, "compile: "+err.Error())
			return
		}
		format := r.URL.Query().Get("format")
		var text string
		switch format {
		case "dot":
			text, err = result.DOT()
		default:
			text, err = result.Mermaid()
		}
		if err != nil {
			writeErr(w, 500, "visualize: "+err.Error())
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Write([]byte(text))
	}
}
