package service

import (
	"context"
	"testing"
	"time"
)

// TestSpiderWaitJobs_Idle 无在跑协程时 WaitJobs 立即返回 true(优雅停机不被空等阻塞)。
func TestSpiderWaitJobs_Idle(t *testing.T) {
	s := NewSpiderService(nil, nil, nil, nil, nil, nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !s.WaitJobs(ctx) {
		t.Fatal("空闲服务 WaitJobs 应立即 true")
	}
}

// TestSpiderWaitJobs_WaitsRunning 在跑协程收尾前 WaitJobs 阻塞, 收尾后/超时后返回。
func TestSpiderWaitJobs_WaitsRunning(t *testing.T) {
	s := NewSpiderService(nil, nil, nil, nil, nil, nil)
	s.jobs.Add(1)
	released := make(chan struct{})
	go func() {
		time.Sleep(50 * time.Millisecond)
		s.jobs.Done()
		close(released)
	}()

	// 超时预算内协程尚未收尾 → false(走进程退出兜底分支)
	ShortCtx, cancelShort := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancelShort()
	if s.WaitJobs(ShortCtx) {
		t.Fatal("协程未收尾时 WaitJobs 应返回 false")
	}

	<-released
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if !s.WaitJobs(ctx) {
		t.Fatal("协程收尾后 WaitJobs 应返回 true")
	}
}

// TestSpiderBgCtx bgCtx 回退与注入语义: 未注入退 Background, 注入后返回同一实例。
func TestSpiderBgCtx(t *testing.T) {
	s := NewSpiderService(nil, nil, nil, nil, nil, nil)
	if s.bgCtx() != context.Background() {
		t.Fatal("未注入 baseCtx 时应退回 context.Background")
	}
	custom, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.SetBaseCtx(custom)
	if s.bgCtx() != custom {
		t.Fatal("注入后 bgCtx 应返回注入实例(取消信号可达后台采集)")
	}
}
