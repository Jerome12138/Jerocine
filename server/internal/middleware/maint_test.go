package middleware

import "testing"

// 维护令牌是"部署脚本能用、别人不能用"的闸门：空配置必须一律拒绝（fail-closed），
// 不能出现"没配 token 反而全放行"的事故。
func TestMaintTokenOK(t *testing.T) {
	cases := []struct {
		name   string
		got    string
		wanted string
		want   bool
	}{
		{"正常匹配", "s3cret", "s3cret", true},
		{"不匹配", "s3cret", "other", false},
		{"未配置(空 wanted)=拒绝", "s3cret", "", false},
		{"未传头(空 got)=拒绝", "", "s3cret", false},
		{"双空=拒绝", "", "", false},
	}
	for _, c := range cases {
		if got := MaintTokenOK(c.got, c.wanted); got != c.want {
			t.Fatalf("%s: MaintTokenOK(%q,%q)=%v, want %v", c.name, c.got, c.wanted, got, c.want)
		}
	}
}
