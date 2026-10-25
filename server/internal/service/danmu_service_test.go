package service

import (
	"testing"

	"server/internal/danmu"
)

// MatchEpisode 表驱动（评审 7.3：单集片特例 / 越界降级 / 集号匹配）。
func TestMatchEpisode(t *testing.T) {
	series := []danmu.Episode{
		{Cid: 101, Title: "第1集"},
		{Cid: 102, Title: "第2集"},
		{Cid: 103, Title: "第3集"},
	}
	movie := []danmu.Episode{
		{Cid: 9001, Title: "正片"},
	}
	movieMulti := []danmu.Episode{
		{Cid: 9001, Title: "预告片"},
		{Cid: 9002, Title: "正片"},
	}
	cases := []struct {
		name     string
		episodes []danmu.Episode
		req      int
		wantCid  int64
		wantOK   bool
	}{
		{"第2集命中", series, 2, 102, true},
		{"第3集命中", series, 3, 103, true},
		{"越界降级", series, 30, 0, false},
		{"集号不存在降级", series, 5, 0, false},
		{"默认第1集", series, 0, 101, true},
		{"电影单集默认第1集", movie, 1, 9001, true},
		{"电影单集任意集号取第1", movie, 99, 9001, true},
		{"多P电影(预告+正片)按序取", movieMulti, 1, 9001, true},
		{"空列表", nil, 1, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := MatchEpisode(c.episodes, c.req)
			if ok != c.wantOK {
				t.Fatalf("ok = %v, want %v", ok, c.wantOK)
			}
			if c.wantOK && got.Cid != c.wantCid {
				t.Fatalf("cid = %d, want %d", got.Cid, c.wantCid)
			}
		})
	}
}
