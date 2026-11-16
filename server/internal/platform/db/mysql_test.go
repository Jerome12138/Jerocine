package db

import "testing"

// TestWithSessionVar 防呆：DSN 追加会话变量的两种形态（有/无 query），
// 以及空值不污染 DSN —— 派生写连接靠它挂 `sql_log_bin=0`（见 InitMySQLNoBinlog）。
func TestWithSessionVar(t *testing.T) {
	cases := []struct{ in, want string }{
		{"user:pw@tcp(h:3306)/db", "user:pw@tcp(h:3306)/db?sql_log_bin=0"},
		{"user:pw@tcp(h:3306)/db?charset=utf8mb4", "user:pw@tcp(h:3306)/db?charset=utf8mb4&sql_log_bin=0"},
		{"user:pw@tcp(h:3306)/db?parseTime=true&loc=Local", "user:pw@tcp(h:3306)/db?parseTime=true&loc=Local&sql_log_bin=0"},
	}
	for _, c := range cases {
		if got := withSessionVar(c.in, "sql_log_bin=0"); got != c.want {
			t.Errorf("withSessionVar(%q) = %q, want %q", c.in, got, c.want)
		}
	}
	// 空 DSN / 空 kv ⇒ 原样返回（不制造 `?` 尾巴）
	if got := withSessionVar("", "sql_log_bin=0"); got != "" {
		t.Errorf("空 DSN 应原样返回，得到 %q", got)
	}
	if got := withSessionVar("dsn", ""); got != "dsn" {
		t.Errorf("空 kv 应原样返回，得到 %q", got)
	}
}
