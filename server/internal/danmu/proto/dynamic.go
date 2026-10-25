// Package danmupb 用 .proto 定义驱动解析 B站 seg.so 弹幕二进制（评审 7.5：不手写 wire 解析）。
//
// danmaku.proto 通过 go:embed 内嵌；运行时用 protocompile 编译为 FileDescriptor，
// 再以 dynamicpb 解析 seg.so（protobuf wire）。字段以 tag 驱动，单字段漂移不拖垮整条。
package danmupb

import (
	"context"
	_ "embed"
	"errors"
	"io"
	"strings"
	"sync"

	"github.com/bufbuild/protocompile"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/types/dynamicpb"
)

//go:embed danmaku.proto
var danmakuProtoSource string

var (
	descOnce sync.Once
	fileDesc protoreflect.FileDescriptor
	descErr  error
)

// descriptor 编译内嵌 proto（进程内只编译一次；失败为环境级问题，直接暴露）。
func descriptor() (protoreflect.FileDescriptor, error) {
	descOnce.Do(func() {
		compiler := protocompile.Compiler{
			Resolver: protocompile.WithStandardImports(&protocompile.SourceResolver{
				Accessor: func(path string) (io.ReadCloser, error) {
					if path == "danmaku.proto" {
						return io.NopCloser(strings.NewReader(danmakuProtoSource)), nil
					}
					return nil, errors.New("unknown proto source: " + path)
				},
			}),
		}
		fileDesc, descErr = func() (protoreflect.FileDescriptor, error) {
			files, err := compiler.Compile(context.Background(), "danmaku.proto")
			if err != nil {
				return nil, err
			}
			if len(files) == 0 {
				return nil, errors.New("protocompile: no file descriptors returned")
			}
			return files[0], nil
		}()
	})
	return fileDesc, descErr
}

// Comment 单条弹幕的对外最小字段。
type Comment struct {
	Content string `json:"text"`  // 弹幕内容
	TimeMS  int64  `json:"time"`  // 出现时间（毫秒，相对视频起点）
	Color   uint32 `json:"color"` // 颜色 RGB888
	Mode    int32  `json:"mode"`  // 1 滚动 / 4 底部 / 5 顶部
}

// ParseSeg 解析 seg.so 响应体为弹幕列表。未知/异常字段按 tag 跳过，不整体失败。
// 依赖外部网络不可用或 proto 编译失败时返回错误（调用方按"无弹幕"降级处理）。
func ParseSeg(body []byte) ([]Comment, error) {
	fd, err := descriptor()
	if err != nil {
		return nil, err
	}
	replyDesc := fd.Messages().ByName("DmSegMobileReply")
	elemDesc := fd.Messages().ByName("DanmakuElem")
	fields := replyDesc.Fields()
	msg := dynamicpb.NewMessage(replyDesc)
	if err := proto.Unmarshal(body, msg); err != nil {
		return nil, err
	}
	elems := msg.Get(fields.ByName("elems")).List()
	out := make([]Comment, 0, elems.Len())
	for i := 0; i < elems.Len(); i++ {
		m := elems.Get(i).Message()
		if m == nil || !m.IsValid() {
			continue
		}
		content := m.Get(elemDesc.Fields().ByName("content")).String()
		if content == "" {
			continue
		}
		out = append(out, Comment{
			Content: content,
			TimeMS:  m.Get(elemDesc.Fields().ByName("progress")).Int(),
			Color:   uint32(m.Get(elemDesc.Fields().ByName("color")).Uint()),
			Mode:    int32(m.Get(elemDesc.Fields().ByName("mode")).Int()),
		})
	}
	return out, nil
}
