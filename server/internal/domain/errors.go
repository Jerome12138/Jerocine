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
)
