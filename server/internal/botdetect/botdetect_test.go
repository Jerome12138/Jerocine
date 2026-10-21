package botdetect

import "testing"

// 真实浏览器 UA 样本(线上分布里占多数的几种), 不应被判机器人 —— 除非有其他信号。
const (
	uaChromeWin = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/150.0.0.0 Safari/537.36"
	uaChromeMac = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/148.0.0.0 Safari/537.36"
	uaTvWebView = "Mozilla/5.0 (Linux; Android 14; Smart TV Pro Build/UTT2.250416.001; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/119.0.6045.141 Mobile Safari/537.36"
)

func TestIsBotUA(t *testing.T) {
	bots := []string{
		"Googlebot/2.1 (+http://www.google.com/bot.html)",
		"Mozilla/5.0 (compatible; Baiduspider/2.0)",
		"curl/8.4.0",
		"python-requests/2.31.0",
		"Go-http-client/1.1",
		"Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/120.0.0.0 Safari/537.36",
		"Mozilla/5.0 (compatible; AhrefsBot/7.0)",
		"PostmanRuntime/7.36.0",
	}
	for _, ua := range bots {
		if !IsBotUA(ua) {
			t.Errorf("应判为机器人: %s", ua)
		}
	}
	// 空 UA 不按 UA 判(交给请求头规则), 真实浏览器/壳 WebView 都不能误伤
	for _, ua := range []string{"", uaChromeWin, uaChromeMac, uaTvWebView} {
		if IsBotUA(ua) {
			t.Errorf("误判为机器人: %q", ua)
		}
	}
}

func TestClassify(t *testing.T) {
	human := Input{
		UA: uaChromeWin, AcceptLanguage: "zh-CN,zh;q=0.9",
		SecFetchMode: "cors", IP: "58.48.225.70", ISP: "电信",
	}
	if got := Classify(human); got != ReasonHuman {
		t.Fatalf("真人被误判: %s", got)
	}
	// 线上僵尸会话: 正常 Chrome UA + 腾讯云 43.x + 缺浏览器头(顺序: header 先于 datacenter)
	zombie := Input{UA: uaChromeMac, IP: "43.134.163.79", ISP: ""}
	if got := Classify(zombie); got != ReasonHeader {
		t.Fatalf("缺头应判 header, got %s", got)
	}
	// 带头但落在云段 ⇒ datacenter
	zombie.AcceptLanguage, zombie.SecFetchMode = "en-US,en;q=0.9", "cors"
	if got := Classify(zombie); got != ReasonDatacenter {
		t.Fatalf("云段应判 datacenter, got %s", got)
	}
	// 云段换成家宽 IP ⇒ 真人
	zombie.IP, zombie.ISP = "112.96.66.1", "联通"
	if got := Classify(zombie); got != ReasonHuman {
		t.Fatalf("家宽带全头应判真人, got %s", got)
	}
	// 客户端自报 webdriver 优先于请求头
	wd := Input{UA: uaChromeWin, WebDriver: true}
	if got := Classify(wd); got != ReasonWebDriver {
		t.Fatalf("webdriver 应优先, got %s", got)
	}
}

func TestIsDatacenter(t *testing.T) {
	cases := []struct {
		ip, isp string
		want    bool
	}{
		{"43.134.163.79", "", true},   // 腾讯云国际段(线上僵尸会话)
		{"43.172.183.210", "", true},  // 同上
		{"47.243.1.1", "", true},      // 阿里云香港
		{"120.24.78.68", "阿里云", true}, // ISP 命中
		{"58.48.225.70", "电信", false}, // 家宽
		{"112.96.66.1", "联通", false},
		{"114.114.114.114", "0", false}, // ISP 为占位 0
		{"", "", false},
		{"not-an-ip", "", false},
		{"2001:db8::1", "", false}, // IPv6 不判
	}
	for _, c := range cases {
		if got := IsDatacenter(c.ip, c.isp); got != c.want {
			t.Errorf("IsDatacenter(%q,%q)=%v want %v", c.ip, c.isp, got, c.want)
		}
	}
}

func TestReasonLabel(t *testing.T) {
	for _, r := range append(AllReasons, ReasonHuman) {
		if r.Label() == "" {
			t.Errorf("原因 %q 缺标签", r)
		}
	}
}

// IsKnownBot 读取历史明细的兜底判定: 请求头/`webdriver` 不入库, 只能靠 UA + IP。
func TestIsKnownBot(t *testing.T) {
	cases := []struct {
		ua, ip string
		want   bool
	}{
		{"Googlebot/2.1", "1.2.3.4", true},                                    // UA 命中
		{"Mozilla/5.0 Chrome/150.0.0.0 Safari/537.36", "43.134.163.79", true}, // 只靠机房 IP(线上僵尸会话)
		{"Mozilla/5.0 Chrome/150.0.0.0 Safari/537.36", "58.48.225.70", false}, // 家宽真人
		{"", "112.96.66.1", false},                                            // 空 UA + 家宽
	}
	for _, c := range cases {
		if got := IsKnownBot(c.ua, c.ip); got != c.want {
			t.Errorf("IsKnownBot(%q,%q)=%v want %v", c.ua, c.ip, got, c.want)
		}
	}
}
