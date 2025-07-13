package api

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type JobStatus string

const (
	JobStatusPending   JobStatus = "pending"
	JobStatusRunning   JobStatus = "running"
	JobStatusCompleted JobStatus = "completed"
	JobStatusFailed    JobStatus = "failed"
)

type Job struct {
	ID         string                 `json:"id"`
	Status     JobStatus              `json:"status"`
	Configs    []string               `json:"-"`
	Results    map[string]CheckResult `json:"results"`
	TotalCount int                    `json:"total_count"`
	DoneCount  int                    `json:"done_count"`
	StartTime  *time.Time             `json:"start_time,omitempty"`
	EndTime    *time.Time             `json:"end_time,omitempty"`
	CreatedAt  time.Time              `json:"created_at"`
	Error      string                 `json:"error,omitempty"`
	mutex      sync.RWMutex           `json:"-"`
}

type TaskData struct {
	JobID   string   `json:"job_id"`
	Configs []string `json:"configs"`
	Index   int      `json:"index"`
}

type MessageResponse struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
}

type ScanRequest struct {
	Configs []string `json:"configs"`
}

type ScanResponse struct {
	JobID string `json:"job_id"`
}

type CheckRequest struct {
	Configs []string `json:"configs"`
}

type CheckResponse struct {
	Results []CheckResult `json:"results"`
}

type CheckResult struct {
	Index          int      `json:"index"`
	Status         string   `json:"status"`
	Delay          int64    `json:"delay_ms"`
	Error          string   `json:"error,omitempty"`
	CheckerNodeTag []string `json:"tag,omitempty"` // Array of worker tags that successfully processed this result
}

type WorkerPoolStatus struct {
	WorkerCount    int    `json:"worker_count"`
	TotalTasks     int64  `json:"total_tasks"`
	CompletedTasks int64  `json:"completed_tasks"`
	FailedTasks    int64  `json:"failed_tasks"`
	QueueLength    int64  `json:"queue_length"`
	IsRunning      bool   `json:"is_running"`
	Uptime         string `json:"uptime"`
}

type BuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	Date      string `json:"date"`
	GoVersion string `json:"go_version"`
	Platform  string `json:"platform"`
}

type HealthResponse struct {
	Status     string           `json:"status"`
	Version    string           `json:"version"`
	Build      BuildInfo        `json:"build"`
	WorkerPool WorkerPoolStatus `json:"worker_pool"`
}

type VersionInfo struct {
	Version   string
	Commit    string
	Date      string
	GoVersion string
	Platform  string
}

func NewJob(configs []string) *Job {
	return &Job{
		ID:         uuid.NewString(),
		Status:     JobStatusPending,
		Configs:    configs,
		Results:    make(map[string]CheckResult, len(configs)),
		TotalCount: len(configs),
		CreatedAt:  time.Now(),
	}
}

func (j *Job) updateStatus(status JobStatus, err error) {
	j.mutex.Lock()
	defer j.mutex.Unlock()

	now := time.Now()
	j.Status = status
	if status != JobStatusRunning {
		j.EndTime = &now
	} else {
		j.StartTime = &now
	}
	if err != nil {
		j.Error = err.Error()
	}
}

func (j *Job) Start() {
	j.updateStatus(JobStatusRunning, nil)
}

func (j *Job) Complete() {
	j.updateStatus(JobStatusCompleted, nil)
}

func (j *Job) Fail(err error) {
	j.updateStatus(JobStatusFailed, err)
}

func (j *Job) Done() {
	j.mutex.Lock()
	j.DoneCount++
	j.mutex.Unlock()
}

func (j *Job) AddResult(configHash string, result CheckResult) {
	j.mutex.Lock()
	j.Results[configHash] = result
	j.DoneCount++
	j.mutex.Unlock()
}

func HashConfig(config string) string {
	hash := sha256.Sum256([]byte(strings.Split(config, "#")[0]))
	return hex.EncodeToString(hash[:])
}
