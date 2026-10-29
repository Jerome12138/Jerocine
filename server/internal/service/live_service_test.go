package service

import (
	"errors"
	"strings"
	"testing"

	"server/internal/domain"
	"server/internal/domain/entity"
)

func TestLiveSourceInputValidate(t *testing.T) {
	on := int8(entity.LiveSourceEnabled)
	off := int8(entity.LiveSourceDisabled)

	// 合法：默认启用
	e, err := LiveSourceInput{Name: "主源", URL: "https://example.com/list.m3u"}.validate()
	if err != nil {
		t.Fatalf("valid input: %v", err)
	}
	if e.State != entity.LiveSourceEnabled || e.Name != "主源" {
		t.Fatalf("default state/name: %+v", e)
	}
	// State 指针生效
	e, err = LiveSourceInput{Name: "a", URL: "http://x/y.m3u8", State: &off, Remark: " 备注 "}.validate()
	if err != nil || e.State != entity.LiveSourceDisabled || e.Remark != "备注" {
		t.Fatalf("state pointer/remark trim: %v %+v", err, e)
	}
	_ = on

	// 名称必填
	if _, err := (LiveSourceInput{Name: " ", URL: "http://x/y"}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("missing name: %v", err)
	}
	// 地址仅 http(s)
	if _, err := (LiveSourceInput{Name: "a", URL: "rtmp://x/y"}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("bad scheme: %v", err)
	}
	// 地址超长（列宽 1024）
	long := "http://x/" + strings.Repeat("a", 1100)
	if _, err := (LiveSourceInput{Name: "a", URL: long}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("url too long: %v", err)
	}
	// 备注超长（列宽 255）
	if _, err := (LiveSourceInput{Name: "a", URL: "http://x", Remark: strings.Repeat("评", 256)}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("remark too long: %v", err)
	}
	// 非法状态值
	bad := int8(7)
	if _, err := (LiveSourceInput{Name: "a", URL: "http://x", State: &bad}).validate(); !errors.Is(err, domain.ErrInvalidArgument) {
		t.Fatalf("bad state: %v", err)
	}
}
