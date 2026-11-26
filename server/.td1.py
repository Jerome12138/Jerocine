import io

# ============ A) taskcenter：Status.UpdatedAt + Def.WatchTimeout/WatchStall ============
p = r'D:\Git\Jerocine\server\internal\taskcenter\registry.go'
s = io.open(p, encoding='utf-8', newline='').read()

a = '''	StartedAt int64 // ms
	ElapsedMs int64
}'''
b = '''	StartedAt int64 // ms
	ElapsedMs int64
	// UpdatedAt 进度最后更新时间（ms）。观察者据此判断"停滞"：长时间不推进就当卡死收尾，
	// 比"固定 3 小时"更贴近真实（豆瓣欠账补查实测要 30 小时，固定上限会误杀）。
	UpdatedAt int64
}'''
assert a in s
s = s.replace(a, b, 1)

c = '''	// Async 该任务的 Run 是"启动即返回"（真正跑完要轮询 Status 才知道）。
	Async bool'''
d = '''	// Async 该任务的 Run 是"启动即返回"（真正跑完要轮询 Status 才知道）。
	Async bool
	// WatchTimeout 异步任务的台账收尾上限（0 = 默认 3h）。按实测时长给足，别误杀：
	// 豆瓣评分同步 ≈1941 部 × 4s ≈ 2.2h；欠账补查 ≈2.8 万部 ≈ 31h。
	WatchTimeout time.Duration
	// WatchStall 进度停滞上限（0 = 默认 30m）：Status.UpdatedAt 超过这么久没动就按失败收尾，
	// 这样"真卡死"能及时发现，而"跑得慢但一直在动"不会被误杀。
	WatchStall time.Duration'''
assert c in s
s = s.replace(c, d, 1)

# 默认值常量 + 取值方法
e = '''// ---- 生效配置合并（单点）----'''
f = '''// 观察者默认值（Def 未指定时生效）。
const (
	DefaultWatchTimeout = 3 * time.Hour
	DefaultWatchStall   = 30 * time.Minute
)

// WatchTimeoutOrDefault 台账收尾上限。
func (d Def) WatchTimeoutOrDefault() time.Duration {
	if d.WatchTimeout > 0 {
		return d.WatchTimeout
	}
	return DefaultWatchTimeout
}

// WatchStallOrDefault 进度停滞上限。
func (d Def) WatchStallOrDefault() time.Duration {
	if d.WatchStall > 0 {
		return d.WatchStall
	}
	return DefaultWatchStall
}

// ---- 生效配置合并（单点）----'''
assert e in s
s = s.replace(e, f, 1)
io.open(p, 'w', encoding='utf-8', newline='').write(s)
print('A) Status.UpdatedAt + Def 超时/停滞 已加')

# ============ B) 豆瓣 Def：给足时长 + 发布 UpdatedAt ============
p2 = r'D:\Git\Jerocine\server\internal\service\task_defs.go'
s2 = io.open(p2, encoding='utf-8', newline='').read()

s2 = s2.replace('''		st := taskcenter.Status{State: taskcenter.StateIdle, Message: p.Message, StartedAt: p.StartedAt}''',
                '''		st := taskcenter.Status{State: taskcenter.StateIdle, Message: p.Message,
			StartedAt: p.StartedAt, UpdatedAt: p.UpdatedAt}''', 1)

s2 = s2.replace('''			Async: true,
			Run: func(ctx context.Context) error {
				if s == nil {
					return fmt.Errorf("影片服务未装配")
				}
				return s.StartDoubanScoreSyncOnce(ctx)
			},''', '''			Async: true,
			// 实测：近半年候选 ≈1941 部 × 4s ≈ 2.2h；给 8h 余量 + 30 分钟停滞判定
			WatchTimeout: 8 * time.Hour,
			WatchStall:   30 * time.Minute,
			Run: func(ctx context.Context) error {
				if s == nil {
					return fmt.Errorf("影片服务未装配")
				}
				return s.StartDoubanScoreSyncOnce(ctx)
			},''', 1)

s2 = s2.replace('''			Async:  true, // 同上：后台批 + 内存进度''',
                '''			Async:  true, // 同上：后台批 + 内存进度
			// 实测欠账 ≈2.8 万部 × 4s ≈ 31h；给 48h 上限，停滞 30 分钟才判失败
			WatchTimeout: 48 * time.Hour,
			WatchStall:   30 * time.Minute,''', 1)
io.open(p2, 'w', encoding='utf-8', newline='').write(s2)
print('B) 豆瓣 Def 时长 + UpdatedAt')