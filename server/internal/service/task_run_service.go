package service

import (
	"context"
	"errors"
	"log"
	"time"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// TaskRunService 任务运行台账: 登记/收尾/查询/统计。
// 台账是尽力而为的观测设施 —— 仓库为 nil(未注入)时写入静默跳过, 绝不干扰采集主流程。
type TaskRunService struct {
	runs repository.TaskRunRepository
}

func NewTaskRunService(runs repository.TaskRunRepository) *TaskRunService {
	return &TaskRunService{runs: runs}
}

// ErrTasksUnavailable 台账仓库未注入(不应出现在正常部署)。
var ErrTasksUnavailable = errors.New("task_run repository not configured")

func (s *TaskRunService) enabled() bool { return s.runs != nil }

// Start 登记一条运行记录, 返回台账 id(仓库 nil 或写入失败返回 0, 调用方不必处理)。
func (s *TaskRunService) Start(kind, taskType string, cronId int64, sourceId, name string, hours int) int64 {
	if !s.enabled() {
		return 0
	}
	now := time.Now().UnixMilli()
	r := &entity.TaskRun{
		Kind: kind, Type: taskType, CronId: cronId, SourceId: sourceId,
		Name: name, Hours: hours, Status: entity.TaskRunRunning,
		StartedAt: now, CreatedAt: now,
	}
	if err := s.runs.Create(context.Background(), r); err != nil {
		log.Printf("task_run: 登记任务失败 kind=%s type=%s name=%s: %v", kind, taskType, name, err)
		return 0
	}
	return r.Id
}

// Finish 收尾一条运行记录。id<=0(登记失败)时忽略。
func (s *TaskRunService) Finish(id int64, status string, total, done, failed int, message, errMsg string) {
	if !s.enabled() || id <= 0 {
		return
	}
	ctx := context.Background()
	cur, gerr := s.runs.Get(ctx, id)
	if gerr != nil || cur == nil {
		return // 行已不存在(清理过), 不再补写
	}
	if len(errMsg) > 512 {
		errMsg = errMsg[:512]
	}
	if len(message) > 255 {
		message = message[:255]
	}
	cur.Status = status
	cur.Total = total
	cur.Done = done
	cur.Failed = failed
	cur.Message = message
	cur.Error = errMsg
	cur.EndedAt = time.Now().UnixMilli()
	if err := s.runs.Update(ctx, cur); err != nil {
		log.Printf("task_run: 收尾任务失败 id=%d: %v", id, err)
	}
}

// Get 取一条运行记录(仓库未注入返回 ErrTasksUnavailable)。
func (s *TaskRunService) Get(ctx context.Context, id int64) (*entity.TaskRun, error) {
	if !s.enabled() {
		return nil, ErrTasksUnavailable
	}
	return s.runs.Get(ctx, id)
}

// List 台账分页(status/type 为空则不过滤)。
func (s *TaskRunService) List(ctx context.Context, status, taskType string, page repository.Page) ([]entity.TaskRun, int64, error) {
	if !s.enabled() {
		return nil, 0, nil
	}
	return s.runs.List(ctx, repository.TaskRunFilter{Status: status, Type: taskType}, page.Normalize(20))
}

// LatestByCron 每个定时任务最近一次运行。
func (s *TaskRunService) LatestByCron(ctx context.Context, cronIds []int64) (map[int64]entity.TaskRun, error) {
	if !s.enabled() {
		return map[int64]entity.TaskRun{}, nil
	}
	return s.runs.LatestByCron(ctx, cronIds)
}

// TaskOverview 任务管理页统计卡。
type TaskOverview struct {
	Running         int   `json:"running"`         // 运行中任务数(活跃采集 job + 台账 running)
	TodaySuccess    int64 `json:"todaySuccess"`    // 今日成功任务数
	TodayFailed     int64 `json:"todayFailed"`     // 今日失败任务数
	PendingFailures int64 `json:"pendingFailures"` // 待补采失败页数
}

// Overview 统计卡。running 由调用方把 Redis 活跃采集 job 数合并进来。
func (s *TaskRunService) Overview(ctx context.Context, runningJobs int, pendingFailures int64) TaskOverview {
	o := TaskOverview{Running: runningJobs, PendingFailures: pendingFailures}
	if !s.enabled() {
		return o
	}
	now := time.Now()
	startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
	succ, fail, err := s.runs.CountSince(ctx, startOfDay.UnixMilli())
	if err == nil {
		o.TodaySuccess, o.TodayFailed = succ, fail
	}
	o.Running += s.countRunning(ctx)
	return o
}

func (s *TaskRunService) countRunning(ctx context.Context) int {
	if !s.enabled() {
		return 0
	}
	n, err := s.runs.CountRunning(ctx)
	if err != nil {
		return 0
	}
	return int(n)
}

// MarkInterrupted 服务重启引导时调用: 把遗留 running 行标记为"服务重启, 任务中断",
// 避免进程重启后台账里出现永远"运行中"的幽灵任务。
func (s *TaskRunService) MarkInterrupted(ctx context.Context) {
	if !s.enabled() {
		return
	}
	if err := s.runs.MarkInterrupted(ctx); err != nil {
		log.Printf("task_run: 标记中断任务失败: %v", err)
	}
}
