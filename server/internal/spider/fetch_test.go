package spider

import (
	"testing"
)

func TestParseDetailJSON(t *testing.T) {
	body := []byte(`{"code":1,"pagecount":10,"total":195,"list":[
		{"vod_id":1,"vod_name":"片A","vod_play_from":"snm3u8","vod_play_url":"第1集$https://cdn.x/a/1.m3u8#第2集$https://cdn.x/a/2.m3u8"},
		{"vod_id":2,"vod_name":"片B"},
		{"vod_id":3,"vod_name":"片C"}]}`)
	films, pageCount, total, sample, err := parseDetailJSON(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if films != 3 || pageCount != 10 || total != 195 {
		t.Fatalf("films=%d pageCount=%d total=%d", films, pageCount, total)
	}
	if sample != "https://cdn.x/a/1.m3u8" {
		t.Fatalf("sample m3u8 = %q", sample)
	}
}

func TestParseDetailJSON_NoM3u8(t *testing.T) {
	body := []byte(`{"code":1,"pagecount":2,"list":[{"vod_id":1,"vod_name":"片A","vod_play_from":"mp4","vod_play_url":"正片$https://x/a.mp4"}]}`)
	_, _, _, sample, err := parseDetailJSON(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if sample != "" {
		t.Fatalf("expected no m3u8 sample, got %q", sample)
	}
}

func TestParseDetailXML(t *testing.T) {
	body := []byte(`<?xml version="1.0" encoding="utf-8"?>
<rss version="5.1"><list page="1" pagecount="5" pagesize="20" recordcount="100">
  <video><id>1</id><name><![CDATA[片A]]></name></video>
  <video><id>2</id><name><![CDATA[片B]]></name></video>
</list></rss>`)
	films, pageCount, err := parseDetailXML(body)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if films != 2 || pageCount != 5 {
		t.Fatalf("films=%d pageCount=%d", films, pageCount)
	}
}

func TestParseDetailMalformed(t *testing.T) {
	if _, _, _, _, err := parseDetailJSON([]byte(`not json`)); err == nil {
		t.Fatal("expected parse error for malformed JSON")
	}
	if _, _, err := parseDetailXML([]byte(`<rss><broken`)); err == nil {
		t.Fatal("expected parse error for malformed XML")
	}
}

// 按片名搜索参数: ac=detail + pg + wd(关键字)。
func TestSearchParams(t *testing.T) {
	p := searchParams(2, "凡人修仙传")
	if p.Get("ac") != "detail" {
		t.Fatalf("ac=%q want detail", p.Get("ac"))
	}
	if p.Get("pg") != "2" {
		t.Fatalf("pg=%q want 2", p.Get("pg"))
	}
	if p.Get("wd") != "凡人修仙传" {
		t.Fatalf("wd=%q want 凡人修仙传", p.Get("wd"))
	}
}

// 按 vodId 精确取详情参数: ac=detail + ids(逗号分隔)。
func TestIdsParams(t *testing.T) {
	p := idsParams("12,34,56")
	if p.Get("ac") != "detail" {
		t.Fatalf("ac=%q want detail", p.Get("ac"))
	}
	if p.Get("ids") != "12,34,56" {
		t.Fatalf("ids=%q want 12,34,56", p.Get("ids"))
	}
	if p.Get("wd") != "" || p.Get("pg") != "" {
		t.Fatalf("ids 精确取片不应带 wd/pg: %v", p)
	}
}

// joinIds: 拼逗号串, 跳过非正数。
func TestJoinIds(t *testing.T) {
	if got := joinIds([]int64{12, 34, 56}); got != "12,34,56" {
		t.Fatalf("joinIds=%q want 12,34,56", got)
	}
	if got := joinIds([]int64{0, 7, -1, 9}); got != "7,9" {
		t.Fatalf("joinIds 应跳过非正数, got %q want 7,9", got)
	}
	if got := joinIds(nil); got != "" {
		t.Fatalf("joinIds(nil)=%q want 空", got)
	}
	if got := joinIds([]int64{0, -5}); got != "" {
		t.Fatalf("全非正数应为空, got %q", got)
	}
}
