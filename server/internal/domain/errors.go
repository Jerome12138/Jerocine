package domain

import "errors"

// 领域错误: service/repository 返回这些哨兵错误, handler 层映射为对应 HTTP 状态码 (RFC7807)。
var (
	ErrNotFound        = errors.New("resource not found")
	ErrMovieNotFound   = errors.New("movie not found")
	ErrUserNotFound    = errors.New("user not found")
	ErrInvalidArgument = errors.New("invalid argument")
	ErrConflict        = errors.New("resource conflict")
	ErrUnauthorized    = errors.New("unauthorized")
	ErrForbidden       = errors.New("forbidden")
	// ErrInvalidSourceID 采集源 id 不符合命名规则。
	// id 是 collect_source 主键, 被 movie_play_source.site_id / collect_failure.source_id /
	// source_health.source_id / cron_task.source_ids 以字符串引用(无外键), 一旦落库不可再改,
	// 因此入库前必须守住格式(见 manage_service.collectSourceIDRe)。
	ErrInvalidSourceID = errors.New("invalid source id: 小写字母开头, 仅小写字母/数字/下划线, 2~32 字符")
	// ErrInvalidMove 轮播位排序请求无法执行: 越界 / 已在边界 / 自动位不可下移(恒在尾部)。
	ErrInvalidMove = errors.New("无法移动：已在边界或目标位无效")
)
