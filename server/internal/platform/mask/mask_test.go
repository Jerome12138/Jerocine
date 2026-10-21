package mask

import "testing"

func TestMiddle(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"空串保持空串(未配置语义)", "", ""},
		{"1 字符", "a", Redacted},
		{"11 字符仍整串打码", "abcdefghijk", Redacted},
		{"12 字符起露首尾各 4", "abcdefghijkl", "abcd…ijkl"},
		{"v3 key 32 位", "0c06ba1f5c8d9e2f4a7b3c6d8e0f1a2b", "0c06…1a2b"},
		{"v4 token 很长", "eyJhbGciOiJIUzI1NiJ9.eyJhIjoxfQ.signature", "eyJh…ture"},
		{"中文凭级按 rune 切不乱码", "这是一个很长的中文密钥字符串", "这是一个…钥字符串"},
		{"13 字符", "supersecurecode", "supe…code"},
	}
	for _, c := range cases {
		if got := Middle(c.in); got != c.want {
			t.Errorf("%s: Middle(%q)=%q, want %q", c.name, c.in, got, c.want)
		}
	}
}

// TestMiddleNeverLeaksMiddle 脱敏结果不得包含原值中间段 —— 防止有人把实现改成
// "整串 + 省略号" 这类假脱敏。
func TestMiddleNeverLeaksMiddle(t *testing.T) {
	const s = "0123456789abcdefghij"
	got := Middle(s)
	if got == s {
		t.Fatal("未脱敏: 返回了明文")
	}
	if len(got) >= len(s) {
		t.Fatalf("脱敏后长度未减少: %q", got)
	}
	for i := 4; i+4 < len(s); i++ {
		if got == s[i-4:i+4] {
			t.Fatalf("疑似命中中间段: %q", got)
		}
	}
}
