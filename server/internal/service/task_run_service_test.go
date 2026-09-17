package service

import (
	"context"
	"testing"
	"time"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// fakeTaskRunRepo 内存台账(单测用)。
type fakeTaskRunRepo struct {
	rows   []entity.TaskRun
	nextId int64
}

func (f *fakeTaskRunRepo) Create(_ context.Context, r *entity.TaskRun) error {
	f.nextId++
	r.Id = f.nextId
	if r.CreatedAt == 0 {
		r.CreatedAt = time.Now().UnixMilli()
	}
	f.rows = append(f.rows, *r)
	return nil
}
func (f *fakeTaskRunRepo) Update(_ context.Context, r *entity.TaskRun) error {
	for i := range f.rows {
		if f.rows[i].Id == r.Id {
			f.rows[i] = *r
			return nil
		}
	}
	return nil
}
func (f *fakeTaskRunRepo) Get(_ context.Context, id int64) (*entity.TaskRun, error) {
	for i := range f.rows {
		if f.rows[i].Id == id {
			r := f.rows[i]
			return &r, nil
		}
	}
	return nil, nil
}
func (f *fakeTaskRunRepo) List(_ context.Context, fl repository.TaskRunFilter, page repository.Page) ([]entity.TaskRun, int64, error) {
	var out []entity.TaskRun
	for _, r := range f.rows {
		if fl.Status != "" && r.Status != fl.Status {
			continue
		}
		if fl.Type != "" && r.Type != fl.Type {
			continue
		}
		out = append(out, r)
	}
	return out, int64(len(out)), nil
}
func (f *fakeTaskRunRepo) LatestByCron(_ context.Context, cronIds []int64) (map[int64]entity.TaskRun, error) {
	m := map[int64]entity.TaskRun{}
	for _, r := range f.rows {
		if r.Kind != entity.TaskRunKindCron {
			continue
		}
		for _, id := range cronIds {
			if r.CronId == id {
				if cur, ok := m[id]; !ok || r.Id > cur.Id {
					m[id] = r
				}
			}
		}
	}
	return m, nil
}
func (f *fakeTaskRunRepo) CountSince(_ context.Context, sinceMs int64) (int64, int64, error) {
	var succ, fail int64
	for _, r := range f.rows {
		if r.StartedAt < sinceMs {
			continue
		}
		switch r.Status {
		case entity.TaskRunSuccess:
			succ++
		case entity.TaskRunFailed:
			fail++
		}
	}
	return succ, fail, nil
}
func (f *fakeTaskRunRepo) CountRunning(_ context.Context) (int64, error) {
	var n int64
	for _, r := range f.rows {
		if r.Status == entity.TaskRunRunning {
			n++
		}
	}
	return n, nil
}
func (f *fakeTaskRunRepo) MarkInterrupted(_ context.Context) error {
	for i := range f.rows {
		if f.rows[i].Status == entity.TaskRunRunning {
			f.rows[i].Status = entity.TaskRunFailed
			f.rows[i].Error = "服务重启, 任务中断"
		}
	}
	return nil
}

// 重启后遗留 running 行标记为中断失败, 不再占运行中统计。
func TestTaskRunService_MarkInterrupted(t *testing.T) {
	repo := &fakeTaskRunRepo{}
	svc := NewTaskRunService(repo)
	ctx := context.Background()

	svc.Start(entity.TaskRunKindCron, entity.TaskRunCollect, 3, "", "定时任务#3", 24)
	svc.MarkInterrupted(ctx)

	if o := svc.Overview(ctx, 0, 0); o.Running != 0 || o.TodayFailed != 1 {
		t.Fatalf("中断后统计不符: %+v", o)
	}
	list, _, _ := svc.List(ctx, entity.TaskRunFailed, "", repository.Page{Current: 1, Size: 20})
	if len(list) != 1 || list[0].Error != "服务重启, 任务中断" {
		t.Fatalf("中断行标记不符: %+v", list)
	}
}

// Start→Finish 生命周期: 收尾后状态/结果/原因可查, 且计入统计。
func TestTaskRunService_Lifecycle(t *testing.T) {
	svc := NewTaskRunService(&fakeTaskRunRepo{})
	ctx := context.Background()

	id := svc.Start(entity.TaskRunKindManual, entity.TaskRunCollect, 0, "src_a", "手动采集：源A", 24)
	if id <= 0 {
		t.Fatal("Start 应返回台账 id")
	}
	r, err := svc.Get(ctx, id)
	if err != nil || r.Status != entity.TaskRunRunning {
		t.Fatalf("登记后应为 running: %+v err=%v", r, err)
	}

	svc.Finish(id, entity.TaskRunFailed, 10, 0, 10, "采集失败", "获取分页总数失败: 连接超时")
	r, _ = svc.Get(ctx, id)
	if r.Status != entity.TaskRunFailed || r.Error == "" || r.EndedAt < r.StartedAt {
		t.Fatalf("收尾信息不完整: %+v", r)
	}

	// 统计: 今天的失败数 = 1, 运行中 = 0
	svc.Start(entity.TaskRunKindCron, entity.TaskRunRecover, 3, "", "定时任务#3", 0)
	o := svc.Overview(ctx, 0, 5)
	if o.TodayFailed != 1 || o.Running != 1 || o.PendingFailures != 5 {
		t.Fatalf("overview 不符: %+v", o)
	}

	// LatestByCron: cron 3 最近一次运行 = recover 那条(仍 running)
	latest, _ := svc.LatestByCron(ctx, []int64{3})
	if lr, ok := latest[3]; !ok || lr.Type != entity.TaskRunRecover {
		t.Fatalf("LatestByCron 不符: %+v", latest)
	}
}

// 仓库为 nil 时全链路静默跳过(不 panic, 不报错)。
func TestTaskRunService_NilRepoNoop(t *testing.T) {
	svc := NewTaskRunService(nil)
	id := svc.Start(entity.TaskRunKindManual, entity.TaskRunCollect, 0, "src_a", "x", 0)
	if id != 0 {
		t.Fatalf("nil 仓库 Start 应返回 0, got %d", id)
	}
	svc.Finish(0, entity.TaskRunSuccess, 0, 0, 0, "", "") // 不 panic
	if _, err := svc.Get(context.Background(), 1); err != ErrTasksUnavailable {
		t.Fatalf("nil 仓库 Get 应报 ErrTasksUnavailable, got %v", err)
	}
	if o := svc.Overview(context.Background(), 2, 3); o.Running != 2 || o.PendingFailures != 3 {
		t.Fatalf("nil 仓库 Overview 应保留注入值: %+v", o)
	}
}

// finishCollectRun / finishRecoverRun 的文案与状态判定。
func TestTaskRunService_FinishHelpers(t *testing.T) {
	svc := NewTaskRunService(&fakeTaskRunRepo{})
	ctx := context.Background()

	// 部分源失败 → failed, 带首个失败原因
	id := svc.Start(entity.TaskRunKindCron, entity.TaskRunCollect, 7, "", "定时任务#7", 24)
	s := &SpiderService{tasks: svc}
	s.finishCollectRun(id, CollectSummary{Sources: 3, OK: 2, Failed: 1, Skipped: 0, Total: 100, Done: 80, FailedPages: 3, FirstErr: "源B 403"})
	r, _ := svc.Get(ctx, id)
	if r.Status != entity.TaskRunFailed || r.Error == "" || r.Failed != 3 {
		t.Fatalf("失败收尾不符: %+v", r)
	}

	// 全部成功 → success
	id2 := svc.Start(entity.TaskRunKindCron, entity.TaskRunCollect, 7, "", "定时任务#7", 24)
	s.finishCollectRun(id2, CollectSummary{Sources: 2, OK: 2, Total: 10, Done: 10})
	r2, _ := svc.Get(ctx, id2)
	if r2.Status != entity.TaskRunSuccess {
		t.Fatalf("成功收尾不符: %+v", r2)
	}

	// 补采仍失败 → failed
	id3 := svc.Start(entity.TaskRunKindManual, entity.TaskRunRecover, 0, "", "失败页补采", 0)
	s.finishRecoverRun(id3, RecoverResult{Scanned: 5, Widened: 2, Replayed: 2, Failed: 1, Busy: 0})
	r3, _ := svc.Get(ctx, id3)
	if r3.Status != entity.TaskRunFailed || r3.Message == "" || r3.Error == "" {
		t.Fatalf("补采失败收尾不符: %+v", r3)
	}
}
