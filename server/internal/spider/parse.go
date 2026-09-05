package spider

import (
	"regexp"
	"strconv"
	"strings"
	"time"

	"server/internal/domain"
	"server/internal/domain/entity"
)

var reYear4 = regexp.MustCompile(`[1-9][0-9]{3}`)

// toMovie 把 maccms 详情映射为领域影片实体。
func toMovie(d filmDetail) *entity.Movie {
	return &entity.Movie{
		Mid: d.VodID, Cid: d.TypeID, Pid: d.TypeID1, Name: d.VodName, SubTitle: d.VodSub,
		CName: d.TypeName, EnName: d.VodEn, Initial: d.VodLetter, ClassTag: d.VodClass,
		Area: d.VodArea, Language: d.VodLang, Year: parseYear(d.VodPubDate, d.VodYear),
		Actor: d.VodActor, Director: d.VodDirector, Writer: d.VodWriter, Content: d.VodContent,
		DbId: d.VodDouBanID, DbScore: parseScore(d.VodDouBanScore, d.VodScore), Hits: d.VodHits,
		State: d.VodState, Remarks: d.VodRemarks, Cover: d.VodPic,
		PlayFrom: splitNonEmpty(d.VodPlayFrom, firstNonEmpty(d.VodPlayNote, "$$$")),
		DownFrom: d.VodDownFrom,
		ReleaseStamp: d.VodTimeAdd, UpdateStamp: parseTimeUnix(d.VodTime),
	}
}

type playGroup struct {
	Name     string
	Episodes []entity.Episode
}

// parsePlayGroups 按分组分隔符 note 把 from/url 对齐(先 zip 再过滤, 避免旧实现过滤后错位),
// 仅保留含 m3u8/mp4 的分组。
func parsePlayGroups(playFrom, playURL, note string) []playGroup {
	if note == "" {
		note = "$$$"
	}
	names := strings.Split(playFrom, note)
	groups := strings.Split(playURL, note)
	out := make([]playGroup, 0, len(groups))
	for i, g := range groups {
		if !strings.Contains(g, ".m3u8") && !strings.Contains(g, ".mp4") {
			continue
		}
		name := ""
		if i < len(names) {
			name = strings.TrimSpace(names[i])
		}
		if name == "" {
			name = "线路" + strconv.Itoa(i+1)
		}
		out = append(out, playGroup{Name: name, Episodes: splitEpisodes(g)})
	}
	return out
}

// splitEpisodes 把一组播放串("名$链接#名$链接")拆成集列表。
func splitEpisodes(group string) []entity.Episode {
	parts := strings.Split(group, "#")
	eps := make([]entity.Episode, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		if i := strings.IndexByte(p, '$'); i >= 0 {
			eps = append(eps, entity.Episode{Episode: p[:i], Link: p[i+1:]})
		} else {
			eps = append(eps, entity.Episode{Episode: "正片", Link: p})
		}
	}
	return eps
}

// buildPlaySources 把一部影片的播放分组映射为多源行。
// master(mid!=nil): 按 mid 关联, match_key=hash(片名)。
// slave(mid==nil): 额外按 hash(dbId) 再存一份以提高跨站匹配率; 跳过"解说"类。
func buildPlaySources(d filmDetail, siteId string, mid *int64) []entity.MoviePlaySource {
	if mid == nil && strings.Contains(d.TypeName, "解说") {
		return nil
	}
	groups := parsePlayGroups(d.VodPlayFrom, d.VodPlayURL, d.VodPlayNote)
	if len(groups) == 0 {
		return nil
	}
	keys := []string{domain.GenerateHashKey(d.VodName)}
	if mid == nil && d.VodDouBanID > 0 {
		keys = append(keys, domain.GenerateHashKey(d.VodDouBanID))
	}
	out := make([]entity.MoviePlaySource, 0, len(groups)*len(keys))
	for _, k := range keys {
		for _, g := range groups {
			out = append(out, entity.MoviePlaySource{
				Mid: mid, SiteId: siteId, MatchKey: k, SourceVodID: d.VodID,
				PlayFrom: g.Name, Episodes: g.Episodes,
			})
		}
	}
	return out
}

// jsonEpisodeCount 取该影片最长线路的集数(供搜索预览展示, 与播放装配口径一致: 仅含 m3u8/mp4 线路)。
func jsonEpisodeCount(d filmDetail) int {
	max := 0
	for _, g := range parsePlayGroups(d.VodPlayFrom, d.VodPlayURL, d.VodPlayNote) {
		if n := len(g.Episodes); n > max {
			max = n
		}
	}
	return max
}

// xmlEpisodeCount 同 jsonEpisodeCount, XML 形态。
func xmlEpisodeCount(v videoDetail) int {
	max := 0
	for _, dd := range v.DL.DD {
		if !strings.Contains(dd.Value, ".m3u8") && !strings.Contains(dd.Value, ".mp4") {
			continue
		}
		if n := len(splitEpisodes(dd.Value)); n > max {
			max = n
		}
	}
	return max
}

// parseCategories 扁平化 maccms 分类(默认展示)。
func parseCategories(list []filmClass) []entity.Category {
	out := make([]entity.Category, 0, len(list))
	for i, c := range list {
		out = append(out, entity.Category{Id: c.TypeID, Pid: c.TypePid, Name: c.TypeName, Show: true, Sort: i})
	}
	return out
}

// ---- XML ----

func xmlToMovie(v videoDetail) *entity.Movie {
	return &entity.Movie{
		Mid: v.ID, Cid: v.Tid, Name: v.Name.Text, CName: v.Type, Cover: v.Pic,
		Language: v.Lang, Area: v.Area, Year: parseYear(v.Year, v.Year), State: v.State,
		Remarks: v.Note.Text, Actor: v.Actor.Text, Director: v.Director.Text, Content: v.Des.Text,
	}
}

func xmlPlaySources(v videoDetail, siteId string, mid *int64) []entity.MoviePlaySource {
	out := make([]entity.MoviePlaySource, 0, len(v.DL.DD))
	key := domain.GenerateHashKey(v.Name.Text)
	for _, dd := range v.DL.DD {
		if !strings.Contains(dd.Value, ".m3u8") && !strings.Contains(dd.Value, ".mp4") {
			continue
		}
		name := dd.Flag
		if name == "" {
			name = "线路"
		}
		out = append(out, entity.MoviePlaySource{
			Mid: mid, SiteId: siteId, MatchKey: key, SourceVodID: v.ID,
			PlayFrom: name, Episodes: splitEpisodes(dd.Value),
		})
	}
	return out
}

// ---- helpers ----

func parseScore(douban, vod string) float64 {
	if f, err := strconv.ParseFloat(strings.TrimSpace(douban), 64); err == nil && f > 0 {
		return f
	}
	if f, err := strconv.ParseFloat(strings.TrimSpace(vod), 64); err == nil {
		return f
	}
	return 0
}

func parseYear(pubDate, fallback string) int {
	if y := reYear4.FindString(pubDate); y != "" {
		n, _ := strconv.Atoi(y)
		return n
	}
	if y := reYear4.FindString(fallback); y != "" {
		n, _ := strconv.Atoi(y)
		return n
	}
	return 0
}

func parseTimeUnix(s string) int64 {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0
	}
	if t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local); err == nil {
		return t.Unix()
	}
	return 0
}

func splitNonEmpty(s, sep string) entity.StringSlice {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, sep)
	out := make(entity.StringSlice, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func firstNonEmpty(a, b string) string {
	if a != "" {
		return a
	}
	return b
}
