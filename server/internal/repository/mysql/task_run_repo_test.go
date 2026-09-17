package mysql

import (
	"context"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"

	"server/internal/domain/entity"
	"server/internal/domain/repository"
)

func TestTaskRunRepo_CreateAndUpdate(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewTaskRunRepository(gdb)

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("INSERT INTO `task_run`")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()

	now := time.Now().UnixMilli()
	r := &entity.TaskRun{
		Kind: entity.TaskRunKindManual, Type: entity.TaskRunCollect,
		SourceId: "src_lz", Name: "手动采集：HD(lz)", Hours: -1,
		Status: entity.TaskRunRunning, StartedAt: now,
	}
	if err := repo.Create(context.Background(), r); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if r.Id != 1 {
		t.Fatalf("want id=1, got %d", r.Id)
	}

	mock.ExpectBegin()
	mock.ExpectExec(regexp.QuoteMeta("UPDATE `task_run`")).
		WillReturnResult(sqlmock.NewResult(1, 1))
	mock.ExpectCommit()
	r.Status = entity.TaskRunSuccess
	r.Total, r.Done, r.Failed = 10, 9, 1
	r.Message = "采集完成, 1 页失败"
	r.EndedAt = time.Now().UnixMilli()
	if err := repo.Update(context.Background(), r); err != nil {
		t.Fatalf("Update: %v", err)
	}
}

func TestTaskRunRepo_LatestByCron(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewTaskRunRepository(gdb)

	now := time.Now().UnixMilli()
	cols := []string{"id", "kind", "task_type", "cron_id", "source_id", "name", "hours", "status", "total", "done", "failed", "message", "error", "started_at", "ended_at", "created_at", "updated_at"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT t.* FROM task_run t")).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(5, entity.TaskRunKindCron, entity.TaskRunCollect, 3, "", "定时任务#3", 24, entity.TaskRunSuccess, 0, 0, 0, "成功 1 个源", "", now, now, now, now))

	m, err := repo.LatestByCron(context.Background(), []int64{3})
	if err != nil {
		t.Fatalf("LatestByCron: %v", err)
	}
	if len(m) != 1 || m[3].Id != 5 || m[3].Status != entity.TaskRunSuccess {
		t.Fatalf("want latest run for cron 3, got %+v", m)
	}
	for _, frag := range []string{"kind = ?", "GROUP BY cron_id"} {
		if !strings.Contains(*lastSQL, frag) {
			t.Fatalf("LatestByCron SQL 缺少 %q: %s", frag, *lastSQL)
		}
	}
}

func TestTaskRunRepo_CountSince(t *testing.T) {
	gdb, mock, _ := newMockDBCapture(t)
	repo := NewTaskRunRepository(gdb)

	mock.ExpectQuery(regexp.QuoteMeta("SELECT SUM(CASE WHEN status = ? THEN 1 ELSE 0 END) AS success")).
		WillReturnRows(sqlmock.NewRows([]string{"success", "failed"}).AddRow(3, 1))

	succ, fail, err := repo.CountSince(context.Background(), 1)
	if err != nil {
		t.Fatalf("CountSince: %v", err)
	}
	if succ != 3 || fail != 1 {
		t.Fatalf("want 3/1, got %d/%d", succ, fail)
	}
}

func TestTaskRunRepo_ListFilter(t *testing.T) {
	gdb, mock, lastSQL := newMockDBCapture(t)
	repo := NewTaskRunRepository(gdb)

	now := time.Now().UnixMilli()
	cols := []string{"id", "kind", "task_type", "cron_id", "source_id", "name", "hours", "status", "total", "done", "failed", "message", "error", "started_at", "ended_at", "created_at", "updated_at"}
	mock.ExpectQuery(regexp.QuoteMeta("SELECT count(*) FROM `task_run`")).
		WillReturnRows(sqlmock.NewRows([]string{"count"}).AddRow(2))
	mock.ExpectQuery(regexp.QuoteMeta("SELECT * FROM `task_run`")).
		WillReturnRows(sqlmock.NewRows(cols).
			AddRow(1, entity.TaskRunKindCron, entity.TaskRunRecover, 2, "", "失败页补采", 0, entity.TaskRunFailed, 0, 0, 0, "", "补采失败", now, now, now, now))

	list, total, err := repo.List(context.Background(),
		repository.TaskRunFilter{Status: entity.TaskRunFailed}, repository.Page{Current: 1, Size: 20})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if total != 2 || len(list) != 1 || list[0].Error == "" {
		t.Fatalf("list mismatch: total=%d len=%d %+v", total, len(list), list)
	}
	if !strings.Contains(*lastSQL, "status = ?") {
		t.Fatalf("List 应按 status 过滤: %s", *lastSQL)
	}
}
