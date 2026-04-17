package appender

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"
)

type Metadata struct {
	Path       string     `json:"path"`
	Expiration *time.Time `json:"expiration"`
}

// Driver 组合了所有接口，提供完整功能
// 实现者可以选择性实现需要的接口
type Driver struct {
	Getter
	Deleter
	Appender
	Finisher
}

type Appender interface {
	Append(ctx context.Context, id string, data []byte, offset int64) error
}

type Append func(ctx context.Context, id string, data []byte, offset int64) error

func (fn Append) Append(ctx context.Context, id string, data []byte, offset int64) error {
	return fn(ctx, id, data, offset)
}

func NewAppender(fn Append) Appender {
	return fn
}

func Stderr() Appender {
	return NewAppender(func(ctx context.Context, id string, data []byte, offset int64) error {
		_, err := fmt.Fprint(os.Stderr, string(data))
		return err
	})
}

func Stdout() Appender {
	return NewAppender(func(ctx context.Context, id string, data []byte, offset int64) error {
		_, err := fmt.Fprint(os.Stdout, string(data))
		return err
	})
}

// Getter 获取对象的元数据和内容
type Getter interface {
	Get(ctx context.Context, id string) (*Metadata, error)
	GetContent(ctx context.Context, id string) (io.ReadCloser, error)
}

// Deleter 删除对象
type Deleter interface {
	Delete(ctx context.Context, id string) error
}

// Finisher 可选的完成接口，用于资源清理
type Finisher interface {
	Finish(ctx context.Context, id string) error
}

func finish(appender Appender, ctx context.Context, id string) error {
	if appender == nil {
		return nil
	}
	if finisher, ok := appender.(Finisher); ok {
		return finisher.Finish(ctx, id)
	}
	return nil
}

func del(appender Appender, ctx context.Context, id string) error {
	if appender == nil {
		return nil
	}
	if cc, ok := appender.(Deleter); ok {
		return cc.Delete(ctx, id)
	}
	return nil
}
