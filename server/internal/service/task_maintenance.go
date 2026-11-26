package service

// 任务中心 · 维护面：周期归一（cron 表达式 → 友好文本）与台账保留清理。
// 拆分自 task_center_service.go（P3-1），行为不变。

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"server/internal/domain/entity"
	"server/internal/taskcenter"
)

// NormalizeCronSchedules 把存量 cron 表达式幂等转换为友好文本（统一时间语义）。
//
//   - 能转：写入 schedule，之后由任务中心统一调度；
//   - 转不动（步长/区间/列表等复杂语法）：**停用并标注**，等人工在任务中心改周期，
//     绝不按旧表达式静默继续跑（避免"看着在跑、实际周期错"）。
func (s *TaskCenterService) NormalizeCronSchedules(ctx context.Context) {
	if s.crons == nil {
		return
	}
	list, err := s.crons.List(ctx)
	if err != nil {
		log.Printf("[task-center] 读取采集定时任务失败: %v", err)
		return
	}
	for i := range list {
		t := list[i]
		if strings.TrimSpace(t.Schedule) != "" {
			continue
		}
		if sched, ok := taskcenter.CronSpecToSchedule(t.Spec); ok {
			t.Schedule = sched
			if uerr := s.crons.Upsert(ctx, &t); uerr != nil {
				log.Printf("[task-center] 周期转换写库失败 #%d: %v", t.Id, uerr)
				continue
			}
			log.Printf("[task-center] 采集任务 #%d 周期已统一: %q → %s", t.Id, t.Spec, sched)
			continue
		}
		if t.State != entity.StateDisabled {
			t.State = entity.StateDisabled
			if !strings.Contains(t.Remark, "周期待人工配置") {
				t.Remark = strings.TrimSpace(t.Remark + "（周期待人工配置）")
			}
			if uerr := s.crons.Upsert(ctx, &t); uerr != nil {
				log.Printf("[task-center] 停用待配置任务失败 #%d: %v", t.Id, uerr)
			}
			log.Printf("[task-center] 采集任务 #%d 周期无法自动转换，已停用待人工配置: spec=%q", t.Id, t.Spec)
		}
	}
}

// PruneHistory 历史台账保留 30 天（分批删除终态记录；running 不动）。
func (s *TaskCenterService) PruneHistory(ctx context.Context) error {
	if s.runs == nil {
		return fmt.Errorf("台账仓储未装配")
	}
	before := time.Now().AddDate(0, 0, -s.retention()).UnixMilli()
	total := int64(0)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		n, err := s.runs.PruneBefore(ctx, before, 5000)
		if err != nil {
			return err
		}
		total += n
		if n == 0 {
			break
		}
	}
	log.Printf("[task-center] 历史台账清理：删除 %d 行（早于 %s）", total, time.UnixMilli(before).Format("2006-01-02"))
	return nil
}
