package service

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"

	"server/internal/botdetect"
)

func newTestOnlineService(t *testing.T) (*OnlineService, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = rdb.Close() })
	return NewOnlineService(rdb), mr
}

func sess(sid, ip string, uid int64, watching bool) OnlineSession {
	// Path 固定给个非空值: PV 现在按"页面访问"计, 无路径的心跳不计 PV
	return OnlineSession{Sid: sid, IP: ip, UID: uid, Watching: watching, Path: "/p"}
}

// hb 模拟一次心跳: 真实链路由 handler 用 botdetect.Classify(UA + 请求头 + IP/ISP)判定,
// 单测没有请求头, 仅按 UA 规则还原(与线上 UA 命中的那部分行为一致)。
func hb(s *OnlineService, ctx context.Context, sess OnlineSession) {
	reason := botdetect.ReasonHuman
	if botdetect.IsBotUA(sess.UA) {
		reason = botdetect.ReasonUA
	}
	s.Heartbeat(ctx, sess, reason)
}

func TestOnlineService_UV_PV_Watching(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	// 同 IP 三个会话(游客) → UV=1, PV=3(每个新会话首跳各计一次页面访问)
	hb(s, ctx, sess("s1", "1.1.1.1", 0, true))
	hb(s, ctx, sess("s2", "1.1.1.1", 0, false))
	hb(s, ctx, sess("s3", "1.1.1.1", 0, true))
	// 登录用户两个会话(uid=7) → UV+1
	hb(s, ctx, sess("s4", "2.2.2.2", 7, true))
	hb(s, ctx, sess("s5", "27.115.1.1", 7, false))
	// 另一游客另一 IP → UV+1
	hb(s, ctx, sess("s6", "4.4.4.4", 0, true))
	o := s.Overview(ctx)
	if o.PV != 6 {
		t.Fatalf("PV=%d want 6", o.PV)
	}
	if o.UV != 3 { // 游客IP1 + 用户7 + 游客IP4
		t.Fatalf("UV=%d want 3", o.UV)
	}
	if o.Watching != 4 { // s1,s3,s4(uid),s6
		t.Fatalf("Watching=%d want 4", o.Watching)
	}
	if len(o.Sessions) != 6 {
		t.Fatalf("明细行数=%d want 6", len(o.Sessions))
	}
}

func TestOnlineService_EmptySIDIgnored(t *testing.T) {
	s, _ := newTestOnlineService(t)
	hb(s, context.Background(), sess("", "1.1.1.1", 0, true))
	if o := s.Overview(context.Background()); o.PV != 0 || o.UV != 0 {
		t.Fatalf("空 sid 不应计数, got %+v", o)
	}
}

func TestOnlineService_ExpiryPurges(t *testing.T) {
	s, _ := newTestOnlineService(t)
	s.window = 20 * time.Millisecond
	hb(s, context.Background(), sess("s1", "1.1.1.1", 0, true))
	if o := s.Overview(context.Background()); o.PV != 1 {
		t.Fatalf("未过期应在线, got PV=%d", o.PV)
	}
	time.Sleep(30 * time.Millisecond)
	if o := s.Overview(context.Background()); o.PV != 0 {
		t.Fatalf("过期会话应被清理, got PV=%d", o.PV)
	}
}

func TestOnlineService_HeartbeatRefreshesFirstSeen(t *testing.T) {
	s, _ := newTestOnlineService(t)
	s.window = 40 * time.Millisecond
	ctx := context.Background()
	hb(s, ctx, sess("s1", "1.1.1.1", 0, true))
	time.Sleep(20 * time.Millisecond)
	hb(s, ctx, sess("s1", "1.1.1.1", 0, true)) // 续命
	time.Sleep(20 * time.Millisecond)
	o := s.Overview(ctx)
	if len(o.Sessions) != 1 {
		t.Fatalf("续命后应仍在线, got %d 行", len(o.Sessions))
	}
	// PV 是"窗口内页面访问次数": 40ms 窗口下首次访问已过窗口 → 0(会话仍在, 两者口径不同)
	if o.PV != 0 {
		t.Fatalf("访问已过窗口, PV 应归零, got %d", o.PV)
	}
	if len(o.Sessions) != 1 || o.Sessions[0].FirstSeen > o.Sessions[0].LastSeen {
		t.Fatalf("firstSeen 应保持首次进入时间且不晚于 lastSeen, got %+v", o.Sessions[0])
	}
}

func TestOnlineService_SortedByLastSeenDesc(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	hb(s, ctx, sess("s1", "1.1.1.1", 0, false))
	time.Sleep(10 * time.Millisecond)
	hb(s, ctx, sess("s2", "2.2.2.2", 0, false))
	o := s.Overview(ctx)
	if len(o.Sessions) != 2 || o.Sessions[0].Sid != "s2" || o.Sessions[1].Sid != "s1" {
		t.Fatalf("应按最近活跃倒序, got %+v", fmt.Sprintf("%+v", o.Sessions))
	}
}

func TestOnlineService_BotUA_Ignored(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	// applebot: 用户实测在明细里见过的爬虫
	hb(s, ctx, OnlineSession{Sid: "b1", IP: "9.9.9.9", UA: "Mozilla/5.0 (compatible; Applebot/0.1; +http://www.apple.com/go/applebot)"})
	// googlebot / bytespider / curl
	hb(s, ctx, OnlineSession{Sid: "b2", IP: "9.9.9.10", UA: "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)"})
	hb(s, ctx, OnlineSession{Sid: "b3", IP: "9.9.9.11", UA: "Mozilla/5.0 (compatible; Bytespider; spider-feedback@bytedance.com)"})
	hb(s, ctx, OnlineSession{Sid: "b4", IP: "9.9.9.12", UA: "curl/8.5.0"})
	// 机器人按 UA 过滤后不计 PV/UV; 正常用户(带路径首跳)计 1
	hb(s, ctx, OnlineSession{Sid: "u1", IP: "8.8.8.8", UA: "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126.0 Safari/537.36", Path: "/p"})
	o := s.Overview(ctx)
	if o.PV != 1 || o.UV != 1 || len(o.Sessions) != 1 {
		t.Fatalf("爬虫应全部忽略, 仅正常浏览器在线: got %+v", o)
	}
	if o.Today.UV != 1 || o.Today.PV != 1 {
		t.Fatalf("爬虫不应计入今日统计: got %+v", o.Today)
	}
}

func TestOnlineService_DailyStats(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	// 两个不同游客 IP(UV=2, PV=2), 其中 1 人观看中
	hb(s, ctx, sess("s1", "1.1.1.1", 0, true))
	hb(s, ctx, sess("s2", "2.2.2.2", 0, false))
	o := s.Overview(ctx)
	if o.Today.UV != 2 || o.Today.PV != 2 {
		t.Fatalf("今日 UV/PV 应累计: got %+v", o.Today)
	}
	// 峰值: 当前在线 2 个会话, 应更新为 2
	if o.Today.Peak != 2 {
		t.Fatalf("今日峰值应为当前会话数 2: got %+v", o.Today)
	}
	// 同 sid 30min 内再次心跳不新增今日 PV; 新 sid 新增
	hb(s, ctx, sess("s1", "1.1.1.1", 0, true))
	hb(s, ctx, sess("s3", "27.115.1.1", 0, true))
	o = s.Overview(ctx)
	if o.Today.UV != 3 || o.Today.PV != 3 {
		t.Fatalf("s1 续心跳不应新增 PV, s3 应新增: got %+v", o.Today)
	}
	if o.Today.Peak != 3 {
		t.Fatalf("峰值应为 3: got %+v", o.Today)
	}
}

func TestOnlineService_SameSidDifferentIP_SeparateRows(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	// 同一设备(sid=s1)先后从两个出口 IP 访问 → 应显示为两行会话
	hb(s, ctx, sess("s1", "1.1.1.1", 0, true))
	hb(s, ctx, sess("s1", "2.2.2.2", 0, false))
	o := s.Overview(ctx)
	if len(o.Sessions) != 2 {
		t.Fatalf("同 sid 不同 IP 应分两行, got %d 行", len(o.Sessions))
	}
	if o.PV != 2 {
		t.Fatalf("PV 应为 2 个会话行, got %d", o.PV)
	}
	// 游客按 IP 去重 → UV 仍为 2
	if o.UV != 2 {
		t.Fatalf("UV 应 2(两 IP), got %d", o.UV)
	}
	// 同一 sid+IP 续心跳仍合并为一行; PV: s1 两行各首跳计 1, 续跳路径不变不新增
	hb(s, ctx, sess("s1", "1.1.1.1", 0, true))
	o = s.Overview(ctx)
	if len(o.Sessions) != 2 || o.PV != 2 {
		t.Fatalf("同 sid+IP 续心跳应仍为 2 行 PV=2, got %d 行 PV=%d", len(o.Sessions), o.PV)
	}
	// 登录用户同 sid 多 IP: 明细分行, UV 按 uid 去重为 1; 两个新会话首跳 PV 再 +2
	hb(s, ctx, sess("u1", "8.8.8.8", 9, true))
	hb(s, ctx, sess("u1", "9.9.9.9", 9, false))
	o = s.Overview(ctx)
	if len(o.Sessions) != 4 {
		t.Fatalf("登录用户多 IP 应分行, got %d 行", len(o.Sessions))
	}
	if o.PV != 4 {
		t.Fatalf("4 个会话首跳各计一次 PV, got %d", o.PV)
	}
	if o.UV != 3 { // 游客两 IP + 登录用户 u1
		t.Fatalf("UV 应 3, got %d", o.UV)
	}
}

func TestOnlineService_BotHitCounted(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	// 正常用户 1 个
	hb(s, ctx, sess("u1", "8.8.8.8", 0, true))
	// 多个爬虫心跳(不同 IP)
	hb(s, ctx, OnlineSession{Sid: "b1", IP: "9.9.9.9", UA: "Mozilla/5.0 (compatible; Googlebot/2.1)"})
	hb(s, ctx, OnlineSession{Sid: "b2", IP: "9.9.9.10", UA: "Mozilla/5.0 (compatible; Applebot/0.1)"})
	hb(s, ctx, OnlineSession{Sid: "b3", IP: "9.9.9.11", UA: "curl/8.5.0"})
	o := s.Overview(ctx)
	// 爬虫不计入 UV/PV/明细
	if o.PV != 1 || o.UV != 1 || len(o.Sessions) != 1 {
		t.Fatalf("爬虫不应计入 UV/PV/明细: got %+v", o)
	}
	// 实时机器人在线源数(90s 窗口去重) = 3
	if o.Bots != 3 {
		t.Fatalf("Bots=%d want 3", o.Bots)
	}
	// 今日机器人请求 PV = 3(每次 bot 心跳 +1)
	if o.Today.BotPV != 3 {
		t.Fatalf("Today.BotPV=%d want 3", o.Today.BotPV)
	}
	// 今日正常 UV/PV 只含正常用户
	if o.Today.UV != 1 || o.Today.PV != 1 {
		t.Fatalf("今日正常统计被爬虫污染: got %+v", o.Today)
	}
	// 同源 bot 续心跳: bots 去重仍 3, botPV 继续 +1
	hb(s, ctx, OnlineSession{Sid: "b1", IP: "9.9.9.9", UA: "Googlebot/2.1"})
	o = s.Overview(ctx)
	if o.Bots != 3 {
		t.Fatalf("同源 bot 续心跳应去重, Bots=%d want 3", o.Bots)
	}
	if o.Today.BotPV != 4 {
		t.Fatalf("BotPV=%d want 4(第4次心跳)", o.Today.BotPV)
	}
}

// 防御过滤: 规则上线前/演进时, act 与按日日志里可能残留爬虫会话(历史数据),
// Overview 与 DailySessions 输出时应再次过滤(写入端已过滤, 此处为兜底)。
func TestOnlineService_DefensiveBotFilter(t *testing.T) {
	s, mr := newTestOnlineService(t)
	ctx := context.Background()
	// 模拟历史残留: 直接写入一条爬虫 UA 的在线会话(绕过 Heartbeat 的写入端过滤)
	botSess := OnlineSession{Sid: "legacy-bot", IP: "9.9.9.9", UA: "Mozilla/5.0 (compatible; Googlebot/2.1)", LastSeen: time.Now().Unix()}
	buf, _ := json.Marshal(botSess)
	nowMs := time.Now().UnixMilli()
	if _, err := mr.ZAdd("jc:online:act", float64(nowMs), "legacy-bot#9.9.9.9"); err != nil {
		t.Fatalf("seed act: %v", err)
	}
	if err := mr.Set("jc:online:info:legacy-bot#9.9.9.9", string(buf)); err != nil {
		t.Fatalf("seed info: %v", err)
	}
	// 正常用户也在线
	hb(s, ctx, sess("u1", "8.8.8.8", 0, true))
	o := s.Overview(ctx)
	if o.PV != 1 || o.UV != 1 || len(o.Sessions) != 1 {
		t.Fatalf("残留爬虫会话应被 Overview 防御过滤: got %+v", o)
	}
	if o.Sessions[0].Sid != "u1" {
		t.Fatalf("仅正常用户应保留: got %+v", o.Sessions)
	}

	// DailySessions 同理: 按日日志里塞一条爬虫会话, 应被过滤
	date := time.Now().Format("20060102")
	if _, err := mr.ZAdd("jc:online:daily:"+date+":log", float64(nowMs), "legacy-bot#9.9.9.9"); err != nil {
		t.Fatalf("seed dailylog: %v", err)
	}
	if err := mr.Set("jc:online:daily:"+date+":info:legacy-bot#9.9.9.9", string(buf)); err != nil {
		t.Fatalf("seed dailyinfo: %v", err)
	}
	sessList := s.DailySessions(ctx, 1)
	if len(sessList) != 1 || sessList[0].Sid != "u1" {
		t.Fatalf("残留爬虫会话应被 DailySessions 防御过滤: got %+v", sessList)
	}
}

// findSession 从明细里按 sid 取一行(找不到直接失败, 避免 nil 断言噪音)。
func findSession(t *testing.T, list []OnlineSession, sid string) OnlineSession {
	t.Helper()
	for _, s := range list {
		if s.Sid == sid {
			return s
		}
	}
	t.Fatalf("明细里没有 sid=%s: %+v", sid, list)
	return OnlineSession{}
}

func pathSess(sid, ip, path string) OnlineSession {
	return OnlineSession{Sid: sid, IP: ip, Path: path}
}

// 页面访问(PV)口径: 同页停留只计首跳; 路径变化逐次计(回访旧页也计, 不去重);
// 客户端 pv=1(刷新/路由打点)同路径也计; 实时窗口过期后归零。
func TestOnlineService_PageViewSemantics(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	hb(s, ctx, sess("s1", "1.1.1.1", 0, false)) // 首跳 +1
	hb(s, ctx, sess("s1", "1.1.1.1", 0, false)) // 同页续跳 不计
	hb(s, ctx, pathSess("s1", "1.1.1.1", "/p")) // 路径没变 不计
	if o := s.Overview(ctx); o.PV != 1 {
		t.Fatalf("同页停留应只计 1 次 PV, got %d", o.PV)
	}
	hb(s, ctx, pathSess("s1", "1.1.1.1", "/filmDetail")) // 切页 +1
	hb(s, ctx, pathSess("s1", "1.1.1.1", "/p"))          // 回到原页 仍 +1(不去重)
	if o := s.Overview(ctx); o.PV != 3 {
		t.Fatalf("路径变化应逐次计 PV(不去重), got %d", o.PV)
	}
	// 客户端标记 pv=1: 同一路径的刷新也计一次
	hb(s, ctx, OnlineSession{Sid: "s1", IP: "1.1.1.1", Path: "/p", PV: true})
	o := s.Overview(ctx)
	if o.PV != 4 {
		t.Fatalf("pv=1 的刷新应计一次, got %d", o.PV)
	}
	if o.Today.PV != 4 {
		t.Fatalf("今日 PV 应同步累计为 4, got %+v", o.Today)
	}
	// 窗口过期后实时 PV 归零, 今日 PV 不回退
	s.window = 1 * time.Millisecond
	time.Sleep(5 * time.Millisecond)
	if o := s.Overview(ctx); o.PV != 0 {
		t.Fatalf("窗口过期实时 PV 应归零, got %d", o.PV)
	}
	if o := s.Overview(ctx); o.Today.PV != 4 {
		t.Fatalf("今日 PV 不随窗口过期回退, got %+v", o.Today)
	}
}

// 访问次数(Visits, 明细列): 距上次心跳 < gap 算同一次访问, >= gap 记一次新的; 跨天从 1 重计。
func TestOnlineService_Visits(t *testing.T) {
	s, _ := newTestOnlineService(t)
	// 会话时间戳是秒级(展示口径), gap 取 1s 才能在同一测试里跨过去
	s.gap = time.Second
	ctx := context.Background()

	hb(s, ctx, sess("s1", "1.1.1.1", 0, true))
	if got := findSession(t, s.Overview(ctx).Sessions, "s1").Visits; got != 1 {
		t.Fatalf("首访 Visits=%d want 1", got)
	}
	hb(s, ctx, sess("s1", "1.1.1.1", 0, true)) // 30s 心跳的常态: 不新增访问次数
	if got := findSession(t, s.Overview(ctx).Sessions, "s1").Visits; got != 1 {
		t.Fatalf("间隔小于 gap 不应新增, Visits=%d want 1", got)
	}
	time.Sleep(1100 * time.Millisecond)
	hb(s, ctx, sess("s1", "1.1.1.1", 0, true))
	if got := findSession(t, s.Overview(ctx).Sessions, "s1").Visits; got != 2 {
		t.Fatalf("超过 gap 应新增一次访问, Visits=%d want 2", got)
	}
}

// 页面路径: 去重 + 按首次出现顺序保留。
func TestOnlineService_PathsDedup(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	hb(s, ctx, pathSess("s1", "1.1.1.1", "/index"))
	hb(s, ctx, pathSess("s1", "1.1.1.1", "/filmDetail"))
	hb(s, ctx, pathSess("s1", "1.1.1.1", "/index")) // 重复
	hb(s, ctx, pathSess("s1", "1.1.1.1", ""))       // 空路径不入列
	row := findSession(t, s.Overview(ctx).Sessions, "s1")
	if len(row.Paths) != 2 || row.Paths[0] != "/index" || row.Paths[1] != "/filmDetail" {
		t.Fatalf("paths 去重/顺序不对: %+v", row.Paths)
	}
	if row.Path != "" {
		t.Fatalf("Path 应保存最近一次心跳的值(本次为空): %+v", row.Path)
	}
}

func TestMergePaths(t *testing.T) {
	if got := mergePaths(nil, ""); len(got) != 0 {
		t.Fatalf("空路径不应入列: %+v", got)
	}
	if got := mergePaths([]string{"/a", "/b"}, "/a"); len(got) != 2 {
		t.Fatalf("重复路径应去重: %+v", got)
	}
	// 超过上限丢最早的, 保留最近 pathsMax 条
	prev := make([]string, 0, pathsMax)
	for i := 0; i < pathsMax; i++ {
		prev = append(prev, "/p"+string(rune('a'+i)))
	}
	got := mergePaths(prev, "/new")
	if len(got) != pathsMax {
		t.Fatalf("应保留 %d 条, got %d", pathsMax, len(got))
	}
	if got[len(got)-1] != "/new" || got[0] != "/pb" {
		t.Fatalf("应丢最早一条并保留最新: %+v", got)
	}
}

// 机器人按原因拆分计数: 合计 = BotPV, 明细与 UV/PV 不受影响。
func TestOnlineService_BotReasons(t *testing.T) {
	s, _ := newTestOnlineService(t)
	ctx := context.Background()
	bot := OnlineSession{Sid: "b1", IP: "9.9.9.9", UA: "Googlebot/2.1"}
	s.Heartbeat(ctx, bot, botdetect.ReasonUA)
	// 线上僵尸会话: 正常 Chrome UA, 只是机房 IP
	zombie := OnlineSession{Sid: "z1", IP: "43.134.163.79", UA: "Mozilla/5.0 Chrome/150.0.0.0 Safari/537.36"}
	s.Heartbeat(ctx, zombie, botdetect.ReasonDatacenter)
	zombie.Sid = "z2"
	s.Heartbeat(ctx, zombie, botdetect.ReasonDatacenter)
	// 真人
	hb(s, ctx, sess("u1", "8.8.8.8", 0, true))

	o := s.Overview(ctx)
	if o.Today.BotPV != 3 {
		t.Fatalf("BotPV=%d want 3", o.Today.BotPV)
	}
	if o.Today.BotReasons[string(botdetect.ReasonUA)] != 1 ||
		o.Today.BotReasons[string(botdetect.ReasonDatacenter)] != 2 {
		t.Fatalf("原因分布不对: %+v", o.Today.BotReasons)
	}
	if o.PV != 1 || len(o.Sessions) != 1 {
		t.Fatalf("机器人不应进明细: %+v", o)
	}
}

// 近7天明细的"访问次数"= 各天之和(跨天去重保留最近一天的明细, 次数累加)。
func TestDailySessions_VisitsSummedAcrossDays(t *testing.T) {
	s, mr := newTestOnlineService(t)
	ctx := context.Background()
	y := time.Now().AddDate(0, 0, -1).Format("20060102")
	prev := OnlineSession{Sid: "s1", IP: "1.1.1.1", Visits: 2, Paths: []string{"/index"},
		FirstSeen: time.Now().Add(-25 * time.Hour).Unix(), LastSeen: time.Now().Add(-24 * time.Hour).Unix()}
	buf, _ := json.Marshal(prev)
	if _, err := mr.ZAdd("jc:online:daily:"+y+":log", float64(prev.LastSeen*1000), "s1#1.1.1.1"); err != nil {
		t.Fatalf("seed log: %v", err)
	}
	if err := mr.Set("jc:online:daily:"+y+":info:s1#1.1.1.1", string(buf)); err != nil {
		t.Fatalf("seed info: %v", err)
	}
	hb(s, ctx, sess("s1", "1.1.1.1", 0, false)) // 今天第 1 次
	list := s.DailySessions(ctx, 2)
	row := findSession(t, list, "s1")
	if row.Visits != 3 {
		t.Fatalf("跨天访问次数应累加为 3, got %d (%+v)", row.Visits, row)
	}
	if !row.Online {
		t.Fatalf("刚心跳过应标记在线: %+v", row)
	}
}

// 跨零点: 实时明细里残留昨天的记录时, 访问次数/首次进入/页面列表都按"今日"重新计。
func TestOnlineService_CrossDayResetsVisits(t *testing.T) {
	s, mr := newTestOnlineService(t)
	ctx := context.Background()
	yesterday := time.Now().AddDate(0, 0, -1)
	prev := OnlineSession{Sid: "s1", IP: "1.1.1.1", Visits: 5, Paths: []string{"/old"},
		FirstSeen: yesterday.Add(-time.Hour).Unix(), LastSeen: yesterday.Unix()}
	buf, _ := json.Marshal(prev)
	if err := mr.Set("jc:online:info:s1#1.1.1.1", string(buf)); err != nil {
		t.Fatalf("seed: %v", err)
	}
	hb(s, ctx, pathSess("s1", "1.1.1.1", "/index"))
	row := findSession(t, s.Overview(ctx).Sessions, "s1")
	if row.Visits != 1 {
		t.Fatalf("跨天应重新计访问次数, got %d", row.Visits)
	}
	if len(row.Paths) != 1 || row.Paths[0] != "/index" {
		t.Fatalf("跨天页面列表应重新计, got %+v", row.Paths)
	}
	if row.FirstSeen <= yesterday.Unix() {
		t.Fatalf("首次进入时间应重置为今天, got %d", row.FirstSeen)
	}
}
