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
