package entity

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
)

// 自定义 JSON 列类型: 实现 database/sql 的 driver.Valuer + sql.Scanner,
// 让 GORM 直接把 Go 切片/结构体存成 MySQL JSON 列, 读回时反序列化。
// 统一的 scanJSON 处理 nil / []byte / string 三种底层来源。

func scanJSON(dst any, src any) error {
	if src == nil {
		return nil
	}
	switch v := src.(type) {
	case []byte:
		if len(v) == 0 {
			return nil
		}
		return json.Unmarshal(v, dst)
	case string:
		if len(v) == 0 {
			return nil
		}
		return json.Unmarshal([]byte(v), dst)
	default:
		return fmt.Errorf("entity: unsupported scan type %T for JSON column", src)
	}
}

func valueJSON(v any) (driver.Value, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return string(b), nil
}

// StringSlice 映射 JSON 数组列 (如 movie.play_from / app_version.whitelist)。
type StringSlice []string

func (s StringSlice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	return valueJSON(s)
}

func (s *StringSlice) Scan(src any) error { return scanJSON(s, src) }

// Episode 单集播放信息。
type Episode struct {
	Episode string `json:"episode"`
	Link    string `json:"link"`
}

// EpisodeList 映射 movie_play_source.episodes_json (整源一行)。
type EpisodeList []Episode

func (e EpisodeList) Value() (driver.Value, error) {
	if e == nil {
		return "[]", nil
	}
	return valueJSON(e)
}

func (e *EpisodeList) Scan(src any) error { return scanJSON(e, src) }

// JSONMap 映射任意对象 JSON 列 (如 telemetry_event.extra)。
type JSONMap map[string]any

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return "{}", nil
	}
	return valueJSON(m)
}

func (m *JSONMap) Scan(src any) error { return scanJSON(m, src) }

// Int64Slice 映射 JSON 整型数组列 (如 cron_task.source_ids 若用 int id; 当前 source_ids 用字符串切片)。
type Int64Slice []int64

func (s Int64Slice) Value() (driver.Value, error) {
	if s == nil {
		return "[]", nil
	}
	return valueJSON(s)
}

func (s *Int64Slice) Scan(src any) error { return scanJSON(s, src) }
