import io

# ③ API：暂停/继续
p = r'D:\Git\Jerocine\server\internal\handler\manage.go'
s = io.open(p, encoding='utf-8', newline='').read()
a = '''// DoubanBatchStop POST /manage/douban/batch/stop'''
b = '''// DoubanBatchPause POST /manage/douban/batch/pause
// 暂停当前豆瓣批次（在下一部之前停下，可 resume 继续；游标落库，重启后也能续）。
func (h *Handlers) DoubanBatchPause(c *gin.Context) {
	dto.OK(c, gin.H{"paused": h.Film.PauseDoubanBatch()})
}

// DoubanBatchResume POST /manage/douban/batch/resume
func (h *Handlers) DoubanBatchResume(c *gin.Context) {
	dto.OK(c, gin.H{"resumed": h.Film.ResumeDoubanBatch()})
}

// DoubanBatchStop POST /manage/douban/batch/stop'''
assert a in s
s = s.replace(a, b, 1)
io.open(p, 'w', encoding='utf-8', newline='').write(s)

p2 = r'D:\Git\Jerocine\server\internal\router\router.go'
s2 = io.open(p2, encoding='utf-8', newline='').read()
a2 = '''		mg.POST("/douban/batch/stop", h.DoubanBatchStop) // 中止当前豆瓣批次（评分同步/欠账补查）'''
b2 = '''		mg.POST("/douban/batch/pause", h.DoubanBatchPause)   // 暂停当前豆瓣批次（可继续）
		mg.POST("/douban/batch/resume", h.DoubanBatchResume) // 继续已暂停的批次
		mg.POST("/douban/batch/stop", h.DoubanBatchStop)     // 中止当前豆瓣批次（不可续）'''
assert a2 in s2
s2 = s2.replace(a2, b2, 1)
io.open(p2, 'w', encoding='utf-8', newline='').write(s2)
print('③ API 已加')

# ④ 停之后清暂停标记（否则下次启动立刻被暂停卡住）
p3 = r'D:\Git\Jerocine\server\internal\service\douban_refresh.go'
s3 = io.open(p3, encoding='utf-8', newline='').read()
s3 = s3.replace('''	cancel()
	log.Printf("[douban] 已请求中止当前批次")''', '''	doubanBatchPaused.Store(false) // 中止后清暂停标记，避免下一次启动立刻被卡住
	cancel()
	log.Printf("[douban] 已请求中止当前批次")''', 1)
# 两个 Start 入口开跑前也清一次暂停标记（含 "停止" 后残留）
s3 = s3.replace('''	bg, cancel := context.WithCancel(context.WithoutCancel(ctx))
	gen := setDoubanBatchCancel(cancel)
	go func() {
		defer doubanBatchBusy.Store(false)
		defer func() { clearDoubanBatchCancel(gen); cancel() }()
		s.runDoubanScoreSyncLocked(bg)
	}()''', '''	bg, cancel := context.WithCancel(context.WithoutCancel(ctx))
	gen := setDoubanBatchCancel(cancel)
	doubanBatchPaused.Store(false)
	go func() {
		defer doubanBatchBusy.Store(false)
		defer func() { clearDoubanBatchCancel(gen); cancel() }()
		s.runDoubanScoreSyncLocked(bg)
	}()''', 1)
s3 = s3.replace('''	bg, cancel := context.WithCancel(context.WithoutCancel(ctx))
	gen := setDoubanBatchCancel(cancel)
	go func() {
		defer func() { clearDoubanBatchCancel(gen); cancel() }()
		s.runDoubanBackfill(bg, targets)
	}()''', '''	bg, cancel := context.WithCancel(context.WithoutCancel(ctx))
	gen := setDoubanBatchCancel(cancel)
	doubanBatchPaused.Store(false)
	go func() {
		defer func() { clearDoubanBatchCancel(gen); cancel() }()
		s.runDoubanBackfill(bg, targets)
	}()''', 1)
io.open(p3, 'w', encoding='utf-8', newline='').write(s3)
print('④ 暂停标记清理')