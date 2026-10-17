package web

// 模板结构校验测试(对齐 HomeCast 模板防线):
//   Go html/template 只校验 {{}} 语法,HTML 标签闭合/嵌套不校验——
//   缺一个标签(template 平衡)能编译通过,渲染后浏览器自动补,
//   页面"看似正常实际损坏",难题定位(历史 12 天没找到)。
//
//   防线:语法解析 + 块级标签开闭平衡 + 渲染冒烟。
//   shell.html 是页面包装箱,空 `{{.Content}}` 渲染必须成功。

import (
	"html/template"
	"path"
	"regexp"
	"strings"
	"testing"
)

func TestTemplatesParseSyntax(t *testing.T) {
	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || path.Ext(name) != ".html" {
			continue
		}
		if _, err := template.New(name).ParseFS(templatesFS, "templates/"+name); err != nil {
			t.Errorf("%s 模板语法错误: %v", name, err)
		}
	}
}

// blockTags 需要收尾成对的块级/容器结构标签
var blockTags = []string{"div", "span", "button", "details", "summary", "select", "textarea", "table", "tr", "td", "th", "form", "label"}

func TestTemplatesHTMLBalance(t *testing.T) {
	openRe := map[string]*regexp.Regexp{}
	for _, tag := range blockTags {
		openRe[tag] = regexp.MustCompile(`<` + tag + `[ >]`)
	}
	closeRe := map[string]*regexp.Regexp{}
	for _, tag := range blockTags {
		closeRe[tag] = regexp.MustCompile(`</` + tag + `>`)
	}

	entries, err := templatesFS.ReadDir("templates")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || path.Ext(name) != ".html" {
			continue
		}
		b, err := templatesFS.ReadFile("templates/" + name)
		if err != nil {
			t.Fatalf("%s 读取失败: %v", name, err)
		}
		content := string(b)

		// 内联样式/脚本里的 HTML 片段(如 <a> 示例)会误计,跳过 script/style 块
		content = stripInlineCode(content)

		for _, tag := range blockTags {
			open := len(openRe[tag].FindAllString(content, -1))
			close := len(closeRe[tag].FindAllString(content, -1))
			if open != close {
				t.Errorf("%s <%s> 开闭不平衡: 开 %d / 闭 %d(Go template 不校验,浏览器自动补闭合位置,页面易损难查)",
					name, tag, open, close)
			}
		}
	}
}

// stripInlineCode 摘除 <script>/<style> 内容,避免模板里的示例标签干扰计数。
// Go 正则不支持反向引用,script/style 分开写。
func stripInlineCode(content string) string {
	scriptRe := regexp.MustCompile(`(?s)<script[^>]*>.*?</script>`)
	styleRe := regexp.MustCompile(`(?s)<style[^>]*>.*?</style>`)
	return styleRe.ReplaceAllString(scriptRe.ReplaceAllString(content, ""), "")
}

// TestTemplatesRenderSmoke shell 空槽渲染必须成功(历史:模板函数缺失导致
// 整页 500)。HomeCast 同款:Funcs 渲染冒烟。数据对齐 shell 的 media/inspector 面板需求。
func TestTemplatesRenderSmoke(t *testing.T) {
	tmpls, err := Templates()
	if err != nil {
		t.Fatalf("Templates(): %v", err)
	}
	// shell.html 用空 Content + 空面板数据渲染(最低配置的完整页面)
	var sb strings.Builder
	err = tmpls.ExecuteTemplate(&sb, "shell.html", struct {
		Content  interface{}
		Projects []string
		Project  *string
	}{Content: template.HTML("")})
	if err != nil {
		t.Fatalf("shell.html 空槽渲染失败: %v", err)
	}
	if !strings.Contains(sb.String(), "vidproc studio") {
		t.Fatalf("shell.html 输出缺标题,渲染不完整")
	}
}