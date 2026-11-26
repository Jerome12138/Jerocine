package service

import (
	"context"
	"strings"
	"testing"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

// fakeRuns 只覆盖守卫需要的几个方法，其余返回零值。
type fakeRuns struct {
	run       entity.TaskRun
	runningN  int64
	gotType   string
	gotCronID int64
	// 任务中心"此刻是否在跑"的读口径（List 取内置最近一条 / LatestByCron 取采集最近一条）
	latest     []entity.TaskRun
	latestCron map[int64]entity.TaskRun
}

func (f *fakeRuns) Create(context.Context, *entity.TaskRun) error { return nil }
func (f *fakeRuns) Update(context.Context, *entity.TaskRun) error { return nil }
func (f *fakeRuns) Get(context.Context, int64) (*entity.TaskRun, error) {
	r := f.run
	return &r, nil
}
func (f *fakeRuns) List(context.Context, repository.TaskRunFilter, repository.Page) ([]entity.TaskRun, int64, error) {
	return f.latest, int64(len(f.latest)), nil
}
func (f *fakeRuns) LatestByCron(context.Context, []int64) (map[int64]entity.TaskRun, error) {
	if f.latestCron != nil {
		return f.latestCron, nil
	}
	return map[int64]entity.TaskRun{}, nil
}
func (f *fakeRuns) CountSince(context.Context, int64) (int64, int64, error) { return 0, 0, nil }
func (f *fakeRuns) CountRunning(context.Context, []string) (int64, error)   { return 0, nil }
func (f *fakeRuns) MarkInterrupted(context.Context) error                   { return nil }
func (f *fakeRuns) PruneBefore(context.Context, int64, int) (int64, error)  { return 0, nil }
func (f *fakeRuns) SetIgnored(context.Context, int64, bool) error           { return nil }
func (f *fakeRuns) CountRunningByType(_ context.Context, typ string, cronID int64) (int64, error) {
	f.gotType, f.gotCronID = typ, cronID
	return f.runningN, nil
}

var _ repository.TaskRunRepository = (*fakeRuns)(nil)

// 重跑必须做状态判断：还在跑的没有"重跑"可言。
func TestRerunRun_RejectsRunning(t *testing.T) {
	f := &fakeRuns{run: entity.TaskRun{Id: 9, Status: entity.TaskRunRunning, Type: "year_verify"}}
	svc := &TaskCenterService{runs: f, reg: newRegistryWithBuiltins()}
	err := svc.RerunRun(context.Background(), 9)
	if err == nil || !strings.Contains(err.Error(), "仍在运行") {
		t.Fatalf("运行中的任务应拒绝重跑, got %v", err)
	}
}

// 重跑必须幂等：同类型已有在执行中的实例时拒绝，重复点击不会叠加。
func TestRerunRun_RejectsWhenSameTaskRunning(t *testing.T) {
	f := &fakeRuns{run: entity.TaskRun{Id: 9, Status: entity.TaskRunFailed, Type: "year_verify"}, runningN: 1}
	svc := &TaskCenterService{runs: f, reg: newRegistryWithBuiltins()}
	err := svc.RerunRun(context.Background(), 9)
	if err == nil || !strings.Contains(err.Error(), "已有执行中的实例") {
		t.Fatalf("同任务在跑应拒绝重跑, got %v", err)
	}
	if f.gotType != "year_verify" {
		t.Fatalf("应把 type 传给幂等判据, got %q", f.gotType)
	}
}

// 定时采集：幂等判据要带 cronId（同类型不同任务互不影响）。
func TestRerunRun_CronPassesCronID(t *testing.T) {
	f := &fakeRuns{run: entity.TaskRun{Id: 9, Status: entity.TaskRunFailed, Type: entity.TaskRunCollect, CronId: 7}, runningN: 1}
	svc := &TaskCenterService{runs: f, reg: newRegistryWithBuiltins()}
	_ = svc.RerunRun(context.Background(), 9)
	if f.gotCronID != 7 {
		t.Fatalf("cron 任务的幂等判据应带 cronId, got %d", f.gotCronID)
	}
}
