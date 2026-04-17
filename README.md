# Appender

## 项目概述
一个专注于**追加上传**的 Go 语言库，提供按行分割、分块上传的能力。

### 核心特性

- ✅ **按行分割**: 保持日志行完整性，不会在行中间分割
- ✅ **分块上传**: 多行数据批量上传，减少网络请求
- ✅ **智能触发**: 按大小或时间间隔触发上传
- ✅ **接口隔离**: 小接口设计，按需实现
- ✅ **函数类型**: 支持用函数直接实现接口
- ✅ **自动资源管理**: Finisher 接口自动释放资源

### 适用场景

- 日志文件实时上传
- 流式数据处理
- 管道数据转发
- 大文件分块上传

## 核心概念

### 1. 按行分割

```go
// 使用 bufio.Reader.ReadBytes('\n') 逐行读取
line, err := reader.ReadBytes('\n')
```

**优势**:
- 保持日志行完整性
- 适用于大多数日志格式
- 便于后续处理和分析

**注意**: 如果一行数据特别长（超过 `uploadChunkSize`），可能会超出预期的分块大小。

### 2. 分块上传

**触发条件**:
- 缓冲区大小达到 `uploadChunkSize`
- 距离上次上传时间超过 `interval`

**优势**:
- 减少网络请求次数
- 降低存储后端压力
- 平衡实时性和性能

### 3. 两种模式

#### StreamUploader


```go
// 从 io.Reader 上传，遇到 EOF 自动停止
uploader := appender.NewStreamUploader(reader, appender)
// 阻塞
uploader.Run(ctx, "id")

// 不阻塞
// go uploader.Run(ctx, "id")
// ...执行别的逻辑
// <- uploader.Done() // 等待追加完毕
```

**特点**:
- 适用于有限数据源（管道、网络流）
- 自动停止，无需手动管理

#### FileFollower

```go
// 跟随文件增长，必须手动停止
follower := appender.NewFileFollower("/path/to/file", appender)
stopCh := make(chan struct{})
go func() {
    // 10后停止
	<- time.After(10 * time.Second)
	close(stopCh)
}()

// 阻塞
follower.Run(ctx, "id", stopCh)

// 不阻塞
// go follower.Run(ctx, "id", stopCh)
// ...执行主要逻辑
// <- uploader.Done() // 等待追加完毕
```

**特点**:
- 适用于持续增长的日志文件：会自动检测文件大小变化，调整读取位置
- 会永久运行，必须手动停止
- 支持断点续传： `RunFrom`


## 架构设计

### 接口隔离原则（ISP）

```go
// 核心接口：只需要实现 Append
type Appender interface {
    Append(ctx context.Context, id string, data []byte, offset int64) error
}

// 可选接口：按需实现
type Getter interface {
    Get(ctx context.Context, id string) (*Metadata, error)
    GetContent(ctx context.Context, id string) (io.ReadCloser, error)
}

type Deleter interface {
    Delete(ctx context.Context, id string) error
}

type Finisher interface {
    Finish(ctx context.Context, id string) error
}
```

**设计优势**:
- StreamUploader/FileFollower 只依赖 `Appender`
- 实现者可以按需实现接口
- 更灵活、更易测试

### 函数类型支持

#### Appender 函数类型

```go
type Append func(ctx context.Context, id string, data []byte, offset int64) error

func (fn Append) Append(ctx context.Context, id string, data []byte, offset int64) error {
    return fn(ctx, id, data, offset)
}

// 便捷函数
func Stdout() Appender {
    return NewAppender(func(ctx context.Context, id string, data []byte, offset int64) error {
        _, err := fmt.Fprint(os.Stdout, string(data))
        return err
    })
}

func Stderr() Appender {
    return NewAppender(func(ctx context.Context, id string, data []byte, offset int64) error {
        _, err := fmt.Fprint(os.Stderr, string(data))
        return err
    })
}
```

**使用示例**:
```go
// 最简单：直接用函数
uploader := appender.NewStreamUploader(reader, appender.Stdout())

// 或者自定义
uploader := appender.NewStreamUploader(reader, appender.Append(func(ctx, id, data, offset) error {
    return writeToDatabase(ctx, id, data)
}))
```

## 接口设计

### Driver 接口（组合类型）

```go
// Driver 组合了所有接口，提供完整功能
type Driver struct {
    Getter
    Deleter
    Appender
    Finisher
}
```


**用途**:
- 向后兼容
- 需要完整功能的场景

#### 已实现驱动列表
- [File Driver](./file_driver.md): 存储为本地文件
- [aliyunoss](./aliyunoss/README.md): 上传到阿里云OSS

### Metadata 结构

```go
type Metadata struct {
    Path       string     `json:"path"`       // 存储路径或访问 URL
    Expiration *time.Time `json:"expiration"` // 过期时间（可选）
}
```

## 使用指南

### 安装
```shell
go get github.com/zdz1715/appender 
```

### 配置选项详解

#### WithUploadChunkSize

```go
// 小文件、低延迟
appender.WithUploadChunkSize(4*1024)  // 4KB

// 大文件、高带宽
appender.WithUploadChunkSize(64*1024) // 64KB
```

#### WithInterval

```go
// 实时性要求高
appender.WithInterval(100*time.Millisecond)

// 平衡性能
appender.WithInterval(500*time.Millisecond)

// 批量处理
appender.WithInterval(5*time.Second)
```

#### WithReadBuffSize

```go
// 默认值通常足够
// 只在需要调整性能时使用
appender.WithReadBuffSize(16*1024)  // 16KB
```

#### WithDesc

```go
// 添加文件头或元数据
appender.WithDesc([]byte("# Log File\n"))
appender.WithDesc([]byte(fmt.Sprintf("Created at: %s\n", time.Now())))
```

## 已知限制

### 1. 并发安全

当前实现**不是并发安全的**，不要在多个 goroutine 中同时调用同一个实例。

### 2. FileFollower 永久运行

FileFollower 会一直运行，必须手动停止。

