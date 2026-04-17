# AliyunOSS Driver - 阿里云 OSS 存储驱动

## 概述

`aliyunoss` 是 `appender` 包的阿里云 OSS 存储驱动，提供按行分割、分块追加上传到阿里云 OSS 的能力。

### 核心特性

- ✅ **追加上传**: 使用 OSS AppendObject API，支持分块追加上传
- ✅ **自动管理**: 自动处理对象键前缀、Content-Type 等
- ✅ **预签名 URL**: 自动生成预签名下载链接
- ✅ **V2 SDK**: 基于阿里云 OSS Go SDK V2

## 安装

```bash
go get github.com/zdz1715/appender/aliyunoss
```

## 快速开始

### 基本使用

```go
import (
    "context"
    "log"
    "os"

    "github.com/zdz1715/appender"
    "github.com/zdz1715/appender/aliyunoss"
)

func main() {
    ctx := context.Background()

    // 创建 OSS 客户端
    cfg := aliyunoss.Config{
        Region:          "oss-cn-hangzhou",
        AccessKeyId:     os.Getenv("ALIYUN_ACCESS_KEY_ID"),
        AccessKeySecret: os.Getenv("ALIYUN_ACCESS_KEY_SECRET"),
        Bucket:          "my-bucket",
        ObjectPrefix:    "logs",        // 可选，对象前缀
        PresignExpire:   24 * time.Hour, // 预签名 URL 有效期，默认 7 天
    }

    client := aliyunoss.NewClient(cfg)

    // 创建 StreamUploader
    uploader := appender.NewStreamUploader(os.Stdin, client)
}
```

### 与 FileFollower 配合

```go
cfg := aliyunoss.Config{
    Region:          "oss-cn-hangzhou",
    AccessKeyId:     os.Getenv("ALIYUN_ACCESS_KEY_ID"),
    AccessKeySecret: os.Getenv("ALIYUN_ACCESS_KEY_SECRET"),
    Bucket:          "my-bucket",
    ObjectPrefix:    "app-logs",
}

client := aliyunoss.NewClient(cfg)

follower := appender.NewFileFollower(
    "/var/log/app.log",
    client,
    appender.WithUploadChunkSize(64*1024),
    appender.WithInterval(1*time.Second),
)
```

## 配置说明

### Config 结构

```go
type Config struct {
    Region          string        // OSS 区域
    AccessKeyId     string        // Access Key ID
    AccessKeySecret string        // Access Key Secret
    Bucket          string        // Bucket 名称
    ObjectPrefix    string        // 对象前缀（可选）
    PresignExpire   time.Duration // 预签名 URL 有效期（可选，默认 7 天）
}
```

### 参数说明

#### Region

阿里云 OSS 区域，例如：

- `oss-cn-hangzhou` - 华东 1（杭州）
- `oss-cn-shanghai` - 华东 2（上海）
- `oss-cn-beijing` - 华北 1（北京）
- `oss-cn-shenzhen` - 华南 1（深圳）
- `oss-cn-guangzhou` - 华南 2（广州）

#### AccessKeyId / AccessKeySecret

阿里云访问凭证，从 [RAM 访问控制](https://ram.console.aliyun.com/manage/ak) 创建。

**建议**:
- 不要将 AK 硬编码到代码中
- 使用环境变量或配置文件
- 为不同环境使用不同的 AK

#### Bucket

OSS Bucket 名称，需要提前在 OSS 控制台创建。

#### ObjectPrefix

对象键前缀，用于组织文件结构。

```go
// 不设置前缀
cfg.ObjectPrefix = ""  // 对象键: "app.log"

// 设置前缀
cfg.ObjectPrefix = "logs"  // 对象键: "logs/app.log"
cfg.ObjectPrefix = "app/2024/01"  // 对象键: "app/2024/01/app.log"
```

**说明**:
- 前缀会自动去除首尾的 `/`
- 最终对象键格式: `{prefix}/{id}`

#### PresignExpire

预签名 URL 的有效期，默认为 7 天（最大值）。

```go
// 设置为 1 天
cfg.PresignExpire = 24 * time.Hour

// 设置为 1 小时
cfg.PresignExpire = 1 * time.Hour
```

**限制**:
- 最大值为 7 天（`7 * 24 * time.Hour`）
- 超过 7 天会被自动调整为 7 天

## API 说明

### 实现的接口

```go
// Appender - 追加数据
func (c *Client) Append(ctx context.Context, id string, data []byte, offset int64) error

// Getter - 获取元数据（返回预签名 URL）
func (c *Client) Get(ctx context.Context, id string) (*appender.Metadata, error)

// GetContent - 获取对象内容
func (c *Client) GetContent(ctx context.Context, id string) (io.ReadCloser, error)

// Deleter - 删除对象
func (c *Client) Delete(ctx context.Context, id string) error

// Finisher - 资源清理（OSS SDK 自动管理连接，此方法为空实现）
func (c *Client) Finish(ctx context.Context, id string) error
```

### 使用方法

#### 追加数据

```go
err := client.Append(ctx, "app.log", []byte("log data\n"), 0)
```

**说明**:
- 使用 OSS `AppendObject` API
- `offset` 参数用于指定追加位置
- 自动设置 `Content-Type` 为 `text/plain; charset=utf-8`

#### 获取元数据

```go
metadata, err := client.Get(ctx, "app.log")
if err != nil {
    log.Fatal(err)
}

fmt.Printf("Download URL: %s\n", metadata.Path)
fmt.Printf("Expires at: %v\n", metadata.Expiration)
```

**说明**:
- 返回预签名的下载 URL
- URL 格式: `https://{bucket}.{endpoint}/{key}?{signature}`
- `Expiration` 为 URL 的过期时间

#### 下载内容

```go
content, err := client.GetContent(ctx, "app.log")
if err != nil {
    log.Fatal(err)
}
defer content.Close()

data, err := io.ReadAll(content)
fmt.Println(string(data))
```

#### 删除对象

```go
err := client.Delete(ctx, "app.log")
if err != nil {
    log.Fatal(err)
}
```

#### 获取 V2 Client

```go
ossClient := client.V2Client()
// 使用 OSS SDK 的其他功能
```

## OSS 追加上传限制

### 限制说明

1. **对象大小**: 单个对象最大 5 GB
3. **存储类型**: 不支持冷归档和深度冷归档类型
4. **追加位置**: 必须按顺序追加，position 必须准确

### 详细限制

[阿里云 OSS 文档](https://help.aliyun.com/zh/oss/user-guide/append-upload-11)

**关键限制**:
- ❌ 不支持冷归档/深度冷归档
- ❌ 不支持上传回调操作
- ❌ 开启对象保留策略的 Bucket 不支持
- ❌ Append 类型对象不支持设置对象保留策略

## 错误处理

### 常见错误

#### 1. 认证错误

```
ErrorCode: AccessDenied
ErrorMessage: The OSS Access Key ID you provided does not exist in our records.
```

**解决**:
- 检查 AccessKeyId 是否正确
- 检查 AccessKeySecret 是否正确
- 确认 AK 有相应权限

#### 2. Bucket 不存在

```
ErrorCode: NoSuchBucket
ErrorMessage: The specified bucket does not exist.
```

**解决**:
- 确认 Bucket 名称正确
- 在 OSS 控制台创建 Bucket

#### 3. 追加位置错误

```
ErrorCode: PositionNotEqualToLength
ErrorMessage: The position is not equal to the object length.
```

**解决**:
- 确保按顺序追加
- 不要跳过中间数据
- 重新开始上传（使用 `Delete` 删除旧对象）

#### 4. 超过限制

```
ErrorCode: EntityTooLarge
ErrorMessage: The object size exceeds the maximum allowed size.
```

**解决**:
- 检查对象大小是否超过 5GB
- 考虑分片存储

## 相关文档

- [阿里云 OSS 文档](https://help.aliyun.com/zh/oss/) - 官方文档
- [阿里云 OSS Go SDK](https://github.com/aliyun/alibabacloud-oss-go-sdk-v2) - SDK 文档
- [追加上传文档](https://help.aliyun.com/zh/oss/user-guide/append-upload-11) - 追加上传说明

## License

MIT License
