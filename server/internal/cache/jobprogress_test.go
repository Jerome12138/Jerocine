package cache

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"
)

// TestJobControl 校验暂停/继续/取消控制面: job 存在性 + 状态前置 + endedAt 打点,
// 均不满足时返回哨兵而非假成功(此前 handler 恒返 {"ok":true}, 管理页假阳性)。
func TestJobControl(t *testing.T) {
	mr := newTestCache(t)
	ctx := context.Background()

	// coordRdb 未接入 → 报错而非静默 ok
	Init(nil, nil)
	if err := JobControl(ctx, "src_x", JobPaused, JobRunning); !errors.Is(err, ErrCoordUnavailable) {
		t.Fatalf("coordRdb==nil 应返回 ErrCoordUnavailable, got %v", err)
	}
	// 重新接入 miniredis
	Init(redis.NewClient(&redis.Options{Addr: mr.Addr()}), redis.NewClient(&redis.Options{Addr: mr.Addr()}))

	// job 从未启动 → ErrJobNotFound
	if err := JobControl(ctx, "src_x", JobPaused, JobRunning); !errors.Is(err, ErrJobNotFound) {
		t.Fatalf("不存在的 job 应 ErrJobNotFound, got %v", err)
	}

	// running → 可暂停; 暂停后不可再暂停(状态前置)
	if err := JobStart(ctx, "src_a", "源A", 10, -1); err != nil {
		t.Fatalf("JobStart: %v", err)
	}
	if err := JobControl(ctx, "src_a", JobPaused, JobRunning); err != nil {
		t.Fatalf("running 应可暂停: %v", err)
	}
	if st := JobReadState(ctx, "src_a"); st != JobPaused {
		t.Fatalf("暂停后状态应为 paused, got %q", st)
	}
	if err := JobControl(ctx, "src_a", JobPaused, JobRunning); !errors.Is(err, ErrJobStateMismatch) {
		t.Fatalf("已暂停的 job 再暂停应 ErrJobStateMismatch, got %v", err)
	}

	// paused → 可继续, 也可取消(取消打 endedAt 供快照冻结用时)
	if err := JobControl(ctx, "src_a", JobRunning, JobPaused); err != nil {
		t.Fatalf("paused 应可继续: %v", err)
	}
	if err := JobControl(ctx, "src_a", JobCanceled, JobRunning, JobPaused); err != nil {
		t.Fatalf("running 应可取消: %v", err)
	}
	snap, err := JobSnapshot(ctx, "src_a")
	if err != nil {
		t.Fatalf("JobSnapshot: %v", err)
	}
	if snap.State != JobCanceled || snap.EndedAt <= 0 {
		t.Fatalf("取消后应带 endedAt, got state=%q endedAt=%d", snap.State, snap.EndedAt)
	}
	// 已结束的 job 不可再暂停/继续
	if err := JobControl(ctx, "src_a", JobRunning, JobPaused); !errors.Is(err, ErrJobStateMismatch) {
		t.Fatalf("已取消的 job 不可继续, got %v", err)
	}
}

// TestJobProgress_EndedAtInSnapshot JobList/fillJob 应把 endedAt 带出,
// 前端据此冻结已结束任务的"用时"(不再每轮轮询虚增)。
func TestJobProgress_EndedAtInSnapshot(t *testing.T) {
	newTestCache(t)
	ctx := context.Background()
	if err := JobStart(ctx, "src_b", "源B", 5, 24); err != nil {
		t.Fatalf("JobStart: %v", err)
	}
	snap, _ := JobSnapshot(ctx, "src_b")
	if snap.EndedAt != 0 {
		t.Fatalf("运行中任务 endedAt 应为 0, got %d", snap.EndedAt)
	}
	JobSetState(ctx, "src_b", JobDone)
	list, err := JobList(ctx)
	if err != nil {
		t.Fatalf("JobList: %v", err)
	}
	if len(list) != 1 || list[0].EndedAt <= 0 {
		t.Fatalf("JobList 应带出 endedAt>0, got %+v", list)
	}
}
