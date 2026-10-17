// jobs.go — 异步渲染任务:POST /api/projects/{id}/render 启动,
// GET /api/render/{jobID} 轮询进度。单进程内存状态(studio 独立进程,
// 无多实例;渲染重活本来就该独占一个进程跑)。
package api

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"vidproc/internal/cmdrender"
	"vidproc/internal/project"
	"vidproc/internal/render/executor"
)

// jobStatus 任务生命周期。
type jobStatus string

const (
	jobQueued  jobStatus = "queued"
	jobRunning jobStatus = "running"
	jobDone    jobStatus = "done"
	jobFailed  jobStatus = "failed"
)

// RenderJob 一次异步渲染任务的对外状态(进度报告 + 结果)。
type RenderJob struct {
	ID         int64     `json:"id"`
	ProjectID  int64     `json:"project_id"`
	Status     jobStatus `json:"status"`
	Percent    float64   `json:"percent"` // 0-100
	Frame      int64     `json:"frame"`
	Speed      float64   `json:"speed"`
	Output     string    `json:"output,omitempty"`
	Error      string    `json:"error,omitempty"`
	SizeMB     float64   `json:"size_mb,omitempty"`
	Duration   string    `json:"duration,omitempty"`
	StartedAt  time.Time `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	Elapsed    string    `json:"elapsed,omitempty"`
}

// jobStore 并发安全的任务表。任务在本进程内生命周期短(渲染几分钟),
// 不做持久化——进程重启后前端轮询 404,提示重新渲染。
type jobStore struct {
	mu   sync.Mutex
	next atomic.Int64
	jobs map[int64]*RenderJob
}

var renderJobs = &jobStore{jobs: make(map[int64]*RenderJob)}

// start 登记任务并返回 ID。
func (s *jobStore) start(projectID int64) int64 {
	id := s.next.Add(1)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.jobs[id] = &RenderJob{
		ID:        id,
		ProjectID: projectID,
		Status:    jobQueued,
		StartedAt: time.Now(),
	}
	return id
}

// get 读取任务快照;不存在返回 nil。
func (s *jobStore) get(id int64) *RenderJob {
	s.mu.Lock()
	defer s.mu.Unlock()
	j, ok := s.jobs[id]
	if !ok {
		return nil
	}
	cp := *j
	return &cp
}

// update 原子更新任务字段(进度回调高频调用)。
func (s *jobStore) update(id int64, fn func(*RenderJob)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if j, ok := s.jobs[id]; ok {
		fn(j)
	}
}

// fail 置任务为失败并记结束时间。
func fail(id int64, msg string, finished time.Time) {
	renderJobs.update(id, func(j *RenderJob) {
		j.Status = jobFailed
		j.Error = msg
		j.FinishedAt = &finished
		j.Elapsed = finished.Sub(j.StartedAt).Round(10 * time.Millisecond).String()
	})
}

// jobCtx 后台任务上下文(请求已结束,不能绑 r.Context())。
// 渲染持续几分钟,用独立超时兜底,进程退出自然取消。
func jobCtx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 30*time.Minute)
}

// launch 后台执行渲染:materialize spec → cmdrender.Render(带进度回调)。
func launch(db *sql.DB, projectID, jobID int64, output string) {
	p, err := project.Get(db, projectID)
	if err != nil {
		fail(jobID, err.Error(), time.Now())
		return
	}

	tmpSpec := filepath.Join(os.TempDir(), fmt.Sprintf("vidproc_job_%d.yaml", jobID))
	if err := project.SaveSpecToFile(tmpSpec, p.SpecYAML); err != nil {
		fail(jobID, err.Error(), time.Now())
		return
	}
	defer os.Remove(tmpSpec)

	if output == "" {
		output = p.OutputPath
	}
	renderJobs.update(jobID, func(j *RenderJob) { j.Status = jobRunning })

	ctx, cancel := jobCtx()
	defer cancel()
	start := time.Now()
	out, renderErr := cmdrender.Render(ctx, tmpSpec, cmdrender.Options{
		Output: output,
		OnProgress: func(ev executor.ProgressEvent) {
			renderJobs.update(jobID, func(j *RenderJob) {
				j.Percent = ev.Percentage
				j.Frame = ev.Frame
				j.Speed = ev.Speed
			})
		},
	})
	finished := time.Now()
	elapsed := finished.Sub(start).Round(10 * time.Millisecond).String()

	finishedAt := finished
	if renderErr != nil {
		fail(jobID, renderErr.Error(), finishedAt)
		return
	}
	renderJobs.update(jobID, func(j *RenderJob) {
		j.Status = jobDone
		j.Output = out.Path
		j.Duration = out.Duration.String()
		j.SizeMB = float64(out.Size) / (1 << 20)
		j.Elapsed = elapsed
		j.Percent = 100
		j.FinishedAt = &finishedAt
	})
}

// RenderStartHandler POST /api/projects/{id}/render → 启动异步任务,返回 job。
func RenderStartHandler(db *sql.DB) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		p, err := getProject(w, r, db)
		if err != nil {
			return
		}
		output := r.URL.Query().Get("output")
		jobID := renderJobs.start(p.ID)
		go launch(db, p.ID, jobID, output)
		writeJSON(w, 202, renderJobs.get(jobID))
	}
}

// RenderStatusHandler GET /api/render/{jobID} → 任务当前状态。
func RenderStatusHandler(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("jobID"), 10, 64)
	if err != nil {
		writeErr(w, 400, "bad job id")
		return
	}
	job := renderJobs.get(id)
	if job == nil {
		writeErr(w, 404, "job not found(进程重启后任务表丢失)")
		return
	}
	writeJSON(w, 200, job)
}