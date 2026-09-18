package geoip

import "testing"

// 用仓库内 data/ip2region.db 做真数据断言(相对 server 目录路径: 包目录 → internal → server)。
const testDB = "../../data/ip2region.db"

func TestSearchKnownIPs(t *testing.T) {
	s, err := newSearcher(testDB)
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	cases := []struct {
		ip   string
		want string // 空 = 只断言非空
	}{
		{"120.24.78.68", "广东省 深圳市"}, // 阿里云深圳(文档示例)
		{"114.114.114.114", ""},          // 江苏南京(仅断言非空)
		{"8.8.8.8", ""},                  // 美国(仅断言非空)
	}
	for _, c := range cases {
		got := s.Search(c.ip)
		if got == "" {
			t.Errorf("%s: got empty", c.ip)
			continue
		}
		if c.want != "" && got != c.want {
			t.Errorf("%s: got %q, want %q", c.ip, got, c.want)
		}
		t.Logf("%s -> %s", c.ip, got)
	}
}

func TestPrivateAndInvalid(t *testing.T) {
	s, err := newSearcher(testDB)
	if err != nil {
		t.Fatalf("load db: %v", err)
	}
	if got := s.Search("192.168.1.1"); got != "内网" {
		t.Errorf("private: got %q, want 内网", got)
	}
	if got := s.Search("127.0.0.1"); got != "内网" {
		t.Errorf("loopback: got %q, want 内网", got)
	}
	if got := s.Search("10.0.0.1"); got != "内网" {
		t.Errorf("private10: got %q, want 内网", got)
	}
	if got := s.Search("not-an-ip"); got != "" {
		t.Errorf("invalid: got %q, want empty", got)
	}
	if got := s.Search(""); got != "" {
		t.Errorf("empty: got %q, want empty", got)
	}
}

func TestFormatRegion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"中国|0|广东省|深圳市|电信", "广东省 深圳市"},
		{"中国|0|湖北省|武汉市|电信", "湖北省 武汉市"},
		{"中国|0|0|0|0", ""},
		{"美国|0|加利福尼亚|洛杉矶|0", "美国 加利福尼亚 洛杉矶"},
		{"中国|0|湖北省|0|移动", "湖北省"},
	}
	for _, c := range cases {
		if got := formatRegion(c.in); got != c.want {
			t.Errorf("format(%q): got %q, want %q", c.in, got, c.want)
		}
	}
}
