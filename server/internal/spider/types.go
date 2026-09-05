// Package spider 采集引擎: 拉取 maccms(苹果CMS)采集站 JSON/XML → 解析 → 映射为领域实体。
// 从旧 plugin/spider + model/collect + conver 平移收敛, 出站请求走 safehttp 防 SSRF。
package spider

import "encoding/xml"

// ---- maccms JSON ----

type commonPage struct {
	Code      int `json:"code"`
	PageCount int `json:"pagecount"`
	Total     int `json:"total"`
}

type filmListPage struct {
	Code  int         `json:"code"`
	List  []filmBrief `json:"list"`
	Class []filmClass `json:"class"`
}

type filmBrief struct {
	VodID   int64  `json:"vod_id"`
	VodName string `json:"vod_name"`
	TypeID  int64  `json:"type_id"`
}

type filmClass struct {
	TypeID   int64  `json:"type_id"`
	TypePid  int64  `json:"type_pid"`
	TypeName string `json:"type_name"`
}

type filmDetailPage struct {
	Code      int          `json:"code"`
	PageCount int          `json:"pagecount"`
	Total     int          `json:"total"`
	List      []filmDetail `json:"list"`
}

// filmDetail maccms 视频详情(仅保留新架构需要的字段)。
type filmDetail struct {
	VodID          int64  `json:"vod_id"`
	TypeID         int64  `json:"type_id"`
	TypeID1        int64  `json:"type_id_1"`
	TypeName       string `json:"type_name"`
	VodName        string `json:"vod_name"`
	VodSub         string `json:"vod_sub"`
	VodEn          string `json:"vod_en"`
	VodLetter      string `json:"vod_letter"`
	VodClass       string `json:"vod_class"`
	VodPic         string `json:"vod_pic"`
	VodActor       string `json:"vod_actor"`
	VodDirector    string `json:"vod_director"`
	VodWriter      string `json:"vod_writer"`
	VodBlurb       string `json:"vod_blurb"`
	VodRemarks     string `json:"vod_remarks"`
	VodPubDate     string `json:"vod_pubdate"`
	VodArea        string `json:"vod_area"`
	VodLang        string `json:"vod_lang"`
	VodYear        string `json:"vod_year"`
	VodState       string `json:"vod_state"`
	VodHits        int64  `json:"vod_hits"`
	VodScore       string `json:"vod_score"`
	VodTime        string `json:"vod_time"`
	VodTimeAdd     int64  `json:"vod_time_add"`
	VodDouBanID    int64  `json:"vod_douban_id"`
	VodDouBanScore string `json:"vod_douban_score"`
	VodContent     string `json:"vod_content"`
	VodPlayFrom    string `json:"vod_play_from"`
	VodPlayNote    string `json:"vod_play_note"`
	VodPlayURL     string `json:"vod_play_url"`
	VodDownFrom    string `json:"vod_down_from"`
}

// ---- maccms XML ----

type cdata struct {
	Text string `xml:",cdata"`
}

type rssDetail struct {
	XMLName xml.Name `xml:"rss"`
	List    struct {
		PageCount int           `xml:"pagecount,attr"`
		Videos    []videoDetail `xml:"video"`
	} `xml:"list"`
}

type videoDetail struct {
	ID       int64  `xml:"id"`
	Tid      int64  `xml:"tid"`
	Name     cdata  `xml:"name"`
	Type     string `xml:"type"`
	Pic      string `xml:"pic"`
	Lang     string `xml:"lang"`
	Area     string `xml:"area"`
	Year     string `xml:"year"`
	State    string `xml:"state"`
	Note     cdata  `xml:"note"`
	Actor    cdata  `xml:"actor"`
	Director cdata  `xml:"director"`
	Des      cdata  `xml:"des"`
	DL       struct {
		DD []struct {
			Flag  string `xml:"flag,attr"`
			Value string `xml:",cdata"`
		} `xml:"dd"`
	} `xml:"dl"`
}
