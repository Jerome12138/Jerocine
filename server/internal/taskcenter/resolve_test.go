package taskcenter

import "testing"

// 合并规则是"页面显示"与"调度器实际执行"的共同真相，必须逐格钉住：
// 一处合并写错的表现是"页面说启用、任务不跑"，没有任何报错，最难排查。
func TestResolve(t *testing.T) {
	def := Def{
		Key: "builtin:x", Enabled: true, DefaultSchedule: "every:30m",
		DefaultIntervalMs: 5000, MinIntervalMs: 2000,
	}
	defOff := def
	defOff.Enabled = false

	t.Run("无用户配置 = 全走默认", func(t *testing.T) {
		rc := Resolve(def, UserConfig{})
		if !rc.Enabled || rc.Schedule != "every:30m" || rc.IntervalMs != 5000 {
			t.Fatalf("默认值未生效: %+v", rc)
		}
		if rc.FromEnabled != "default" || rc.FromSchedule != "default" {
			t.Fatalf("来源标记应为 default: %+v", rc)
		}
	})

	t.Run("用户停用生效（默认启用的任务）", func(t *testing.T) {
		off := false
		rc := Resolve(def, UserConfig{Enabled: &off})
		if rc.Enabled {
			t.Fatal("用户停用应生效")
		}
		if rc.FromEnabled != "user" {
			t.Fatalf("来源应是 user: %+v", rc)
		}
	})

	t.Run("默认停用的任务，只改备注不该被打开", func(t *testing.T) {
		rc := Resolve(defOff, UserConfig{Remark: "补查用"})
		if rc.Enabled {
			t.Fatal("只改备注不能把默认停用的任务打开")
		}
		if rc.Remark != "补查用" {
			t.Fatalf("备注应透传: %+v", rc)
		}
	})

	t.Run("周期空串=不改（保留默认）", func(t *testing.T) {
		rc := Resolve(def, UserConfig{Schedule: ""})
		if rc.Schedule != "every:30m" {
			t.Fatalf("空周期应保留默认: %+v", rc)
		}
	})

	t.Run("用户周期覆盖", func(t *testing.T) {
		rc := Resolve(def, UserConfig{Schedule: "daily:04:00"})
		if rc.Schedule != "daily:04:00" || rc.FromSchedule != "user" {
			t.Fatalf("用户周期应覆盖: %+v", rc)
		}
	})

	t.Run("间隔：用户值高于下限则生效", func(t *testing.T) {
		rc := Resolve(def, UserConfig{IntervalMs: 3000})
		if rc.IntervalMs != 3000 || rc.FromIntervalMs != "user" {
			t.Fatalf("用户间隔应生效: %+v", rc)
		}
	})

	t.Run("间隔：低于下限一律夹到下限（比默认值和用户值都硬）", func(t *testing.T) {
		rc := Resolve(def, UserConfig{IntervalMs: 500})
		if rc.IntervalMs != 2000 {
			t.Fatalf("应夹到下限 2000, got %d", rc.IntervalMs)
		}
		// 默认值本身低于下限时也要夹（防止注册表写错下限）
		bad := def
		bad.DefaultIntervalMs = 100
		if rc := Resolve(bad, UserConfig{}); rc.IntervalMs != 2000 {
			t.Fatalf("默认值低于下限也应夹, got %d", rc.IntervalMs)
		}
	})
}

// 状态机迁移表：与 cache.JobControl 的前置条件保持一致（service 包有跨层等价性用例）。
func TestNextJobState(t *testing.T) {
	cases := []struct {
		cur  string
		act  JobAction
		want string
		ok   bool
	}{
		{JobRunning, JobActionPause, JobPaused, true},
		{JobRunning, JobActionResume, "", false},
		{JobPaused, JobActionResume, JobRunning, true},
		{JobPaused, JobActionPause, "", false},
		{JobRunning, JobActionCancel, JobCanceled, true},
		{JobPaused, JobActionCancel, JobCanceled, true},
		{JobDone, JobActionCancel, "", false},
		{JobError, JobActionResume, "", false},
		{JobCanceled, JobActionPause, "", false},
	}
	for _, c := range cases {
		got, ok := NextJobState(c.cur, c.act)
		if got != c.want || ok != c.ok {
			t.Fatalf("NextJobState(%s,%s) = (%q,%v), want (%q,%v)", c.cur, c.act, got, ok, c.want, c.ok)
		}
	}
	if !JobTerminal(JobDone) || !JobTerminal(JobError) || !JobTerminal(JobCanceled) ||
		JobTerminal(JobRunning) || JobTerminal(JobPaused) {
		t.Fatal("终态判定不对")
	}
}

// 引用键：构造与解析必须成对（拼法单点，避免各处 "cron:"+itoa 漂移）。
func TestTaskRefs(t *testing.T) {
	if got := RefCron(7); got != "cron:7" {
		t.Fatalf("RefCron = %q", got)
	}
	if got := RefJob("src_lz"); got != "job:src_lz" {
		t.Fatalf("RefJob = %q", got)
	}
	if id, ok := ParseCronRef("cron:7"); !ok || id != 7 {
		t.Fatalf("ParseCronRef(cron:7) = (%d,%v)", id, ok)
	}
	for _, bad := range []string{"cron:", "cron:0", "cron:-1", "cron:abc", "builtin:x", "7", ""} {
		if id, ok := ParseCronRef(bad); ok {
			t.Fatalf("%q 不该被解析成采集任务 id=%d", bad, id)
		}
	}
	// Field 常量与前端/后端口径一致（改了要同步 task_setting 的白名单语义）
	if FieldEnabled != "enabled" || FieldSchedule != "schedule" || FieldRemark != "remark" || FieldInterval != "interval" {
		t.Fatal("Field 常量取值被改动，需同步前后端与文档")
	}
}
