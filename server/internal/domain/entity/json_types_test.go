package entity

import (
	"testing"
)

func TestStringSliceRoundTrip(t *testing.T) {
	in := StringSlice{"正片", "高清"}
	v, err := in.Value()
	if err != nil {
		t.Fatalf("Value err: %v", err)
	}
	s, ok := v.(string)
	if !ok {
		t.Fatalf("Value type = %T, want string", v)
	}
	var out StringSlice
	if err := out.Scan([]byte(s)); err != nil {
		t.Fatalf("Scan err: %v", err)
	}
	if len(out) != 2 || out[0] != "正片" || out[1] != "高清" {
		t.Fatalf("round trip mismatch: %#v", out)
	}
}

func TestStringSliceNilValue(t *testing.T) {
	var in StringSlice
	v, err := in.Value()
	if err != nil {
		t.Fatalf("Value err: %v", err)
	}
	if v.(string) != "[]" {
		t.Fatalf("nil slice Value = %v, want []", v)
	}
}

func TestScanNilAndEmpty(t *testing.T) {
	var out StringSlice
	if err := out.Scan(nil); err != nil {
		t.Fatalf("Scan(nil) err: %v", err)
	}
	if out != nil {
		t.Fatalf("Scan(nil) should leave nil, got %#v", out)
	}
	if err := out.Scan([]byte("")); err != nil {
		t.Fatalf("Scan(empty) err: %v", err)
	}
}

func TestScanString(t *testing.T) {
	var out StringSlice
	if err := out.Scan(`["x"]`); err != nil { // string 来源 (非 []byte)
		t.Fatalf("Scan(string) err: %v", err)
	}
	if len(out) != 1 || out[0] != "x" {
		t.Fatalf("Scan(string) = %#v", out)
	}
}

func TestEpisodeListRoundTrip(t *testing.T) {
	in := EpisodeList{{Episode: "第01集", Link: "http://a/1.m3u8"}, {Episode: "第02集", Link: "http://a/2.m3u8"}}
	v, err := in.Value()
	if err != nil {
		t.Fatalf("Value err: %v", err)
	}
	var out EpisodeList
	if err := out.Scan([]byte(v.(string))); err != nil {
		t.Fatalf("Scan err: %v", err)
	}
	if len(out) != 2 || out[1].Episode != "第02集" || out[1].Link != "http://a/2.m3u8" {
		t.Fatalf("round trip mismatch: %#v", out)
	}
}

func TestJSONMapRoundTrip(t *testing.T) {
	in := JSONMap{"k": "v", "n": float64(3)}
	v, err := in.Value()
	if err != nil {
		t.Fatalf("Value err: %v", err)
	}
	var out JSONMap
	if err := out.Scan([]byte(v.(string))); err != nil {
		t.Fatalf("Scan err: %v", err)
	}
	if out["k"] != "v" || out["n"].(float64) != 3 {
		t.Fatalf("round trip mismatch: %#v", out)
	}
}

func TestUnsupportedScanType(t *testing.T) {
	var out StringSlice
	if err := out.Scan(123); err == nil {
		t.Fatalf("Scan(int) should error")
	}
}
