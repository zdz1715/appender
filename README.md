# Appender

按行分割数据流，然后分块追加上传到存储驱动的 Go 包。

## 概述

`appender` 包提供了从数据源（io.Reader 或文件）按行读取数据，并分块追加到存储后端的能力。核心特点是**按行分割**和**分块上传**：

- **按行分割**：使用 `ReadBytes('\n')` 按行读取数据，保持日志行完整性
- **分块上传**：将多行数据缓存到指定大小后批量上传，减少网络请求
- **智能触发**：支持按大小或时间间隔触发上传，确保实时性和效率的平衡

支持两种模式：

1. **流上传（StreamUploader）**：从 io.Reader 持续读取直到 EOF，**自动停止**
2. **文件跟随（FileFollower）**：跟随文件增长，持续读取新增内容，**必须手动停止**

**重要区别**：
- **StreamUploader**：适合有限数据源（如管道、网络流），遇到 EOF 会自动停止
- **FileFollower**：适合持续增长的日志文件，会永久运行直到手动停止

## 使用
```shell 
go get github.com/zdz1715/appender
```

## 核心概念

### Driver 接口

存储后端抽象，定义了四个方法：

```go
type Driver interface {
    Get(ctx context.Context, id string) (*Metadata, error)
    GetContent(ctx context.Context, id string) (io.ReadCloser, error)
    Delete(ctx context.Context, id string) error
    Append(ctx context.Context, id string, data []byte, offset int64) error
}
```

- `Get`: 获取已上传对象的元数据（路径、过期时间等）
- `GetContent`: 获取已上传对象的内容流
- `Delete`: 删除指定 ID 的对象（上传前会先删除旧数据）
- `Append`: 追加数据块，offset 用于标识数据位置

### Metadata 结构体

```go
type Metadata struct {
    Path       string     `json:"path"`       // 存储路径或访问 URL
    Expiration *time.Time `json:"expiration"` // 过期时间（可选）
}
```

## 使用示例

### 1. StreamUploader - 从 io.Reader 上传

适用于从管道、网络连接等数据源上传：

```go
import (
    "context"
    "os"
    "time"

    "github.com/zdz1715/appender"
)

func exampleStreamUploader() {
    ctx := context.Background()

    // 假设有一个实现了 Driver 接口的存储后端
    var storage appender.Driver

    // 创建 StreamUploader（从 os.Stdin 读取）
    uploader := appender.NewStreamUploader(
        os.Stdin,
        storage,
        appender.WithUploadChunkSize(4*1024),  // 4KB 分块
        appender.WithReadBuffSize(16*1024),    // 16KB 读缓冲
        appender.WithInterval(500*time.Millisecond), // 500ms 上传间隔
        appender.WithDesc([]byte("# Log file header\n")), // 添加描述信息
    )

    // 启动上传，遇到 EOF 自动停止
    err := uploader.Run(ctx, "log-20240416")
    if err != nil {
        // 处理错误
    }
}
```

### 2. FileFollower - 跟随文件增长

适用于实时上传正在增长的日志文件。

**重要**：FileFollower 会一直运行，持续检查文件大小变化并刷新，**必须手动停止**，否则会永久阻塞。

```go
func exampleFileFollower() {
    ctx := context.Background()
    stopCh := make(chan struct{})
    
    var storage appender.Driver
    
    // 创建 FileFollower
    follower := appender.NewFileFollower(
        "/var/log/app.log",
        storage,
        appender.WithUploadChunkSize(64*1024),  // 64KB 分块
        appender.WithInterval(1*time.Second),   // 每秒检查一次
    )
    
    // 从文件开头开始上传
    go follower.Run(ctx, "app-log", stopCh)
    
    // ⚠️ 重要：必须手动停止 FileFollower
    // 方式 1: 关闭 stop channel（推荐）
    // close(stopCh)
    // <-follower.Done() // 等待上传完成
    
    // 方式 2: 取消 context
    // cancel()
    // <-follower.Done()
    
    // 方式 3: 从指定偏移量开始（断点续传）
    // go follower.RunFrom(ctx, "app-log", 1024, stopCh)
}
```

### 3. ConsoleDriver - 控制台输出（调试用）

用于开发和调试，将数据直接输出到 io.Writer：

```go
func exampleConsoleDriver() {
    ctx := context.Background()
    
    // 创建 ConsoleDriver（输出到 stdout）
    driver := appender.NewConsoleDriver(os.Stdout)
    
    err := driver.Append(ctx, "test-id", []byte("Hello, World!\n"), 0)
    if err != nil {
        // 处理错误
    }
}
```

### 4. AliyunOSS Driver - 阿里云 OSS 存储

本包提供了完整的阿里云 OSS Driver 实现，位于 `aliyunoss` 子目录。

#### 安装依赖

```bash
go get github.com/zdz1715/appender/aliyunoss
```

#### 使用示例

```go
import (
    "context"
    "time"
    
    "github.com/zdz1715/appender"
    "github.com/zdz1715/appender/aliyunoss"
)

func exampleAliyunOSS() {
    ctx := context.Background()
    
    // 创建 OSS 客户端
    cfg := aliyunoss.Config{
        Region:          "oss-cn-hangzhou",
        AccessKeyId:     "your-access-key-id",
        AccessKeySecret: "your-access-key-secret",
        Bucket:          "your-bucket-name",
        ObjectPrefix:    "logs",        // 可选，对象前缀
        PresignExpire:   time.Hour * 24 * 7, // 预签名 URL 有效期，默认 7 天
    }
    
    client := aliyunoss.NewClient(cfg)
    
    // 创建 FileFollower，上传日志文件到 OSS
    follower := appender.NewFileFollower(
        "/var/log/app.log",
        client,
        appender.WithUploadChunkSize(64*1024),  // 64KB 分块
        appender.WithInterval(1*time.Second),   // 每秒检查一次
    )
    
    stopCh := make(chan struct{})
    go follower.Run(ctx, "app-20240416.log", stopCh)
    
    // 获取上传对象的访问信息
    metadata, err := client.Get(ctx, "app-20240416.log")
    if err != nil {
        // 处理错误
    }
    // metadata.Path 包含预签名的访问 URL
    // metadata.Expiration 是 URL 的过期时间
    
    // 获取对象内容
    content, err := client.GetContent(ctx, "app-20240416.log")
    if err != nil {
        // 处理错误
    }
    defer content.Close()
    // 读取 content...
    
    // 停止上传
    // close(stopCh)
    // <-follower.Done()
}
```

**阿里云 OSS 追加上传限制**：

[阿里云OSS文档](https://help.aliyun.com/zh/oss/user-guide/append-upload-11)
- Object大小不能超过5 GB。
- 操作限制 
  - 不支持通过追加上传的方式上传冷归档、深度冷归档类型的Object。 
  - 追加上传不支持上传回调操作。 
  - 如果Bucket已开启对象级别保留策略（ObjectWorm），不支持使用追加上传方式，此外，Append类型的Object也不支持设置ObjectWorm。

## 工作流程

1. **按行读取**：使用 `bufio.Reader.ReadBytes('\n')` 逐行读取数据
2. **行缓存**：将读取的行累积到内存缓冲区
3. **批量上传**：当满足以下任一条件时触发上传：
   - 缓冲区大小达到 `uploadChunkSize`
   - 距离上次上传时间超过 `interval`

**FileFollower 特殊说明**：
- 会**永久运行**，持续检查文件大小变化
- 即使文件到达 EOF，也会每隔 `interval` 时间重新检查
- **必须手动停止**：`close(stopCh)` 或取消 context
- 能够处理文件被截断的情况（如日志轮转），会自动调整读取位置
## 配置选项

### WithUploadChunkSize(chunkSize int)

设置上传分块大小（字节）。当缓冲区达到此大小时触发上传。

默认值：1024 字节

### WithReadBuffSize(buffSize int)

设置读缓冲区大小（字节）。

默认值：4096 字节

### WithInterval(interval time.Duration)

设置上传间隔。距离上次上传超过此时间时触发上传。

默认值：500 毫秒

### WithDesc(desc []byte)

设置描述信息，会在上传开始时首先发送（可用于添加文件头、元数据等）。

默认值：空
## 注意事项
### 并发安全

当前实现**不是并发安全的**，不要在多个 goroutine 中同时调用同一个实例的方法。

### 性能考虑

- `uploadChunkSize` 和 `interval` 的选择会影响性能和网络请求频率
- **按行分割**意味着实际分块大小可能略大于 `uploadChunkSize`（会完成当前行）

## 最佳实践

- **分块大小**：通常 4KB-64KB 是合理范围
- **上传间隔**：实时性要求高的场景使用 100-500ms
- **断点续传**：使用 FileFollower 的 `RunFrom` 方法
- **确保停止**：使用 `defer close(stopCh)` 或监听系统信号