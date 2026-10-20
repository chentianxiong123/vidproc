package api

import (
	"encoding/json"
	"net/http"
	"runtime"
	"time"

	"vidproc/internal/ffmpeg"
)

// 版本常量(与前端 About 弹窗 vp.version 保持一致;发布时同步更新)。
const AppVersion = "0.1.0"

// GitHub 仓库(公开)latest release 兜底检查;无 release 时 latest=current,不报更新。
const ghRepo = "chentianxiong123/vidproc"

type versionCheckResponse struct {
	CurrentVersion string  `json:"current_version"`
	LatestVersion  string  `json:"latest_version"`
	HasUpdate      bool    `json:"has_update"`
	ReleaseName    string  `json:"release_name"`
	ReleaseNotes   string  `json:"release_notes"`
	ReleaseURL     string  `json:"release_url"`
	PublishedAt    string  `json:"published_at"`
	Diagnostics    sysDiag `json:"diagnostics"`
	CheckedAt      string  `json:"checked_at"`
}

type sysDiag struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	CPUCount     int    `json:"cpu_count"`
	FFmpeg       string `json:"ffmpeg_version"`
	FFprobe      string `json:"ffprobe_version"`
	GoVersion    string `json:"go_version"`
}

// ghRelease GitHub API 响应子集。
type ghRelease struct {
	TagName     string `json:"tag_name"`
	Name        string `json:"name"`
	Body        string `json:"body"`
	HTMLURL     string `json:"html_url"`
	PublishedAt string `json:"published_at"`
}

func collectSysDiag() sysDiag {
	d := sysDiag{OS: runtime.GOOS, Architecture: runtime.GOARCH, CPUCount: runtime.NumCPU(), GoVersion: runtime.Version()}
	ff, err := ffmpeg.NewConfig()
	if err == nil {
		d.FFmpeg = ff.Version()
	}
	return d
}

// isNewerVersion 语义化比较(参考 useAppUpdate.isNewerVersion)。
func isNewerVersion(remote, current string) bool {
	split := func(v string) []int {
		out := []int{}
		for _, part := range splitVersion(v) {
			out = append(out, atoiSafe(part))
		}
		for len(out) < 3 {
			out = append(out, 0)
		}
		return out
	}
	r, c := split(remote), split(current)
	for i := range 3 {
		if r[i] != c[i] {
			return r[i] > c[i]
		}
	}
	return false
}

func splitVersion(v string) []string {
	out := []string{}
	cur := ""
	for _, ch := range v {
		if ch >= '0' && ch <= '9' {
			cur += string(ch)
		} else if cur != "" {
			out = append(out, cur)
			cur = ""
		}
	}
	if cur != "" {
		out = append(out, cur)
	}
	return out
}

func atoiSafe(s string) int {
	n := 0
	for _, ch := range s {
		if ch < '0' || ch > '9' {
			break
		}
		n = n*10 + int(ch-'0')
	}
	return n
}

// VersionCheckHandler GET /api/version/check?force=true
// 检查 GitHub latest release(5s 超时);失败或无不带 v 前缀的 release 时 latest=current。
func VersionCheckHandler() http.HandlerFunc {
	client := &http.Client{Timeout: 5 * time.Second}
	return func(w http.ResponseWriter, r *http.Request) {
		resp := versionCheckResponse{
			CurrentVersion: AppVersion,
			LatestVersion:  AppVersion,
			HasUpdate:      false,
			ReleaseURL:     "https://github.com/" + ghRepo + "/releases",
			Diagnostics:    collectSysDiag(),
			CheckedAt:      time.Now().UTC().Format(time.RFC3339),
		}
		gh := ghRelease{}
		url := "https://api.github.com/repos/" + ghRepo + "/releases/latest"
		req, err := http.NewRequest(http.MethodGet, url, nil)
		if err == nil {
			req.Header.Set("Accept", "application/vnd.github.v3+json")
			req.Header.Set("User-Agent", "vidproc/"+AppVersion)
			if res, err := client.Do(req); err == nil {
				defer res.Body.Close()
				if res.StatusCode == http.StatusOK {
					_ = json.NewDecoder(res.Body).Decode(&gh)
				}
			}
		}
		if gh.TagName != "" {
			latest := gh.TagName
			if len(latest) > 0 && latest[0] == 'v' {
				latest = latest[1:]
			}
			resp.LatestVersion = latest
			resp.HasUpdate = isNewerVersion(latest, AppVersion)
			resp.ReleaseName = gh.Name
			resp.ReleaseNotes = gh.Body
			resp.ReleaseURL = gh.HTMLURL
			resp.PublishedAt = gh.PublishedAt
		}
		writeJSON(w, 200, resp)
	}
}
