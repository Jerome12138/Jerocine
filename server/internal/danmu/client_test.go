package danmu

import "testing"

// ParsePlatformResource 表驱动：vendors app scheme → 平台资源 id。
func TestParsePlatformResource(t *testing.T) {
	cases := []struct {
		name     string
		uri      string
		provider string
		id       string
		ok       bool
	}{
		{"腾讯 txvideo cid", "txvideo://v.qq.com/VideoDetailActivity?universalStyle=true&cid=mzc002008v8rka3&from=xx", "tencent", "mzc002008v8rka3", true},
		{"腾讯 http 页面带 cid query", "https://v.qq.com/x/cover/cover.html?cid=mzc002008v8rka3&vid=j0047xyz", "tencent", "mzc002008v8rka3", true},
		{"爱奇艺 iqiyi tvid", "iqiyi://mobile/player?aid=203326001&tvid=498189700&ftype=27", "iqiyi", "498189700", true},
		{"优酷 youku showid", "youku://play?source=00003036&showid=efcdfcb66fd346f9b2a9&refer=x", "youku", "efcdfcb66fd346f9b2a9", true},
		{"B站 bilibili season", "bilibili://pgc/season/44847?h5awaken=x", "bilibili", "44847", true},
		{"无 query 的腾讯", "txvideo://v.qq.com/VideoDetailActivity", "tencent", "", false},
		{"无 tvid 的爱奇艺", "iqiyi://mobile/player", "iqiyi", "", false},
		{"未知协议", "mgtv://play?vid=1", "", "", false},
		{"空串", "", "", "", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := ParsePlatformResource(c.uri)
			if ok != c.ok {
				t.Fatalf("ok = %v, want %v", ok, c.ok)
			}
			if !ok {
				return
			}
			if got.Provider != c.provider || got.ResourceID != c.id {
				t.Fatalf("got %+v, want provider=%s id=%s", got, c.provider, c.id)
			}
		})
	}
}

// ParseEpisodeNo 表驱动：B站混乱标题 → 集号。
func TestParseEpisodeNo(t *testing.T) {
	cases := []struct {
		title string
		want  int
	}{
		{"第3集", 3},
		{"第 03 集", 3},
		{"第10话", 10},
		{"第1回", 1},
		{"EP3", 3},
		{"ep03", 3},
		{"EP 12", 12},
		{"3", 3},
		{"30", 30},
		{"第三集", 3},
		{"第十二话", 12},
		{"第二十三集", 23},
		{"第一百零二集", 102},
		{"III", 3},
		{"VIII", 8},
		{"正片", 0},
		{"预告片", 0},
		{"第1集 上", 1}, // 后缀忽略（前缀匹配）
		{"", 0},
		{"无集号标题", 0},
	}
	for _, c := range cases {
		t.Run(c.title, func(t *testing.T) {
			if got := ParseEpisodeNo(c.title); got != c.want {
				t.Fatalf("ParseEpisodeNo(%q) = %d, want %d", c.title, got, c.want)
			}
		})
	}
}
