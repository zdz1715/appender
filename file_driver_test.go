package appender

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestNewFileDriver(t *testing.T) {
	// 创建临时目录
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}

	if driver == nil {
		t.Fatal("Expected non-nil driver")
	}

	if err := driver.Close(); err != nil {
		t.Errorf("Close failed: %v", err)
	}
}

func TestFileDriverAppendAndGet(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}
	defer driver.Close()

	id := "test-file.log"
	data := []byte("line 1\nline 2\nline 3\n")

	// 追加数据
	err = driver.Append(ctx, id, data, 0)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// 获取元数据
	metadata, err := driver.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get failed: %v", err)
	}

	if metadata.Path == "" {
		t.Error("Expected non-empty path")
	}

	// 获取内容
	content, err := driver.GetContent(ctx, id)
	if err != nil {
		t.Fatalf("GetContent failed: %v", err)
	}
	defer content.Close()

	readData, err := io.ReadAll(content)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(readData) != string(data) {
		t.Errorf("Expected data %q, got %q", string(data), string(readData))
	}
}

func TestFileDriverMultipleAppends(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}
	defer driver.Close()

	id := "multi-append.log"

	// 多次追加
	appends := [][]byte{
		[]byte("line 1\n"),
		[]byte("line 2\n"),
		[]byte("line 3\n"),
	}

	for i, data := range appends {
		err = driver.Append(ctx, id, data, int64(i))
		if err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}

	// 验证内容
	content, err := driver.GetContent(ctx, id)
	if err != nil {
		t.Fatalf("GetContent failed: %v", err)
	}
	defer content.Close()

	readData, err := io.ReadAll(content)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	expected := string(appends[0]) + string(appends[1]) + string(appends[2])
	if string(readData) != expected {
		t.Errorf("Expected data %q, got %q", expected, string(readData))
	}
}

func TestFileDriverDelete(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}
	defer driver.Close()

	id := "delete-test.log"
	data := []byte("test data\n")

	// 创建文件
	err = driver.Append(ctx, id, data, 0)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// 验证文件存在
	_, err = driver.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get before delete failed: %v", err)
	}

	// 删除文件
	err = driver.Delete(ctx, id)
	if err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// 验证文件不存在
	_, err = driver.Get(ctx, id)
	if err == nil {
		t.Error("Expected error after delete, got nil")
	}
}

func TestFileDriverClose(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}

	// 写入一些数据
	id := "close-test.log"
	data := []byte("test data\n")
	err = driver.Append(ctx, id, data, 0)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// 关闭驱动
	err = driver.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}

	// 关闭后再写入应该创建新文件句柄
	err = driver.Append(ctx, id, []byte("more data\n"), 0)
	if err != nil {
		t.Errorf("Append after close failed: %v", err)
	}

	// 验证数据
	content, err := driver.GetContent(ctx, id)
	if err != nil {
		t.Fatalf("GetContent failed: %v", err)
	}
	defer content.Close()

	readData, err := io.ReadAll(content)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	expected := "test data\nmore data\n"
	if string(readData) != expected {
		t.Errorf("Expected data %q, got %q", expected, string(readData))
	}
}

func TestFileDriverWithStreamUploader(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}
	defer driver.Close()

	input := "line 1\nline 2\nline 3\n"
	reader := strings.NewReader(input)

	uploader := NewStreamUploader(
		reader,
		driver,
		WithUploadChunkSize(5),
		WithInterval(100*time.Millisecond),
	)

	err = uploader.Run(ctx, "stream-test.log")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	// 验证上传完成后文件句柄已被释放
	if _, ok := driver.files.Load("stream-test.log"); ok {
		t.Error("Expected file handle to be released after upload completes")
	}

	// 验证上传的内容
	content, err := driver.GetContent(ctx, "stream-test.log")
	if err != nil {
		t.Fatalf("GetContent failed: %v", err)
	}
	defer content.Close()

	readData, err := io.ReadAll(content)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	if string(readData) != input {
		t.Errorf("Expected data %q, got %q", input, string(readData))
	}
}

func TestFileDriverCreateDirectory(t *testing.T) {
	// 测试目录不存在时自动创建
	tmpDir := filepath.Join(os.TempDir(), "appender-test", time.Now().Format("20060102-150405"))
	defer os.RemoveAll(tmpDir) // 清理

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}
	defer driver.Close()

	// 验证目录已创建
	info, err := os.Stat(tmpDir)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if !info.IsDir() {
		t.Error("Expected directory to be created")
	}
}

func TestFileDriverCaching(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}
	defer driver.Close()

	id := "cache-test.log"

	// 多次追加到同一个文件
	for i := 0; i < 10; i++ {
		data := []byte(fmt.Sprintf("line %d\n", i))
		err = driver.Append(ctx, id, data, 0)
		if err != nil {
			t.Fatalf("Append %d failed: %v", i, err)
		}
	}

	// 验证所有数据都被写入
	content, err := driver.GetContent(ctx, id)
	if err != nil {
		t.Fatalf("GetContent failed: %v", err)
	}
	defer content.Close()

	readData, err := io.ReadAll(content)
	if err != nil {
		t.Fatalf("ReadAll failed: %v", err)
	}

	expected := ""
	for i := 0; i < 10; i++ {
		expected += fmt.Sprintf("line %d\n", i)
	}

	if string(readData) != expected {
		t.Errorf("Expected data %q, got %q", expected, string(readData))
	}
}

func TestFileDriverConcurrentAppends(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}
	defer driver.Close()

	id := "concurrent-test.log"

	// 并发追加
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			data := []byte(fmt.Sprintf("goroutine %d\n", idx))
			_ = driver.Append(ctx, id, data, 0)
		}(i)
	}
	wg.Wait()

	// 验证文件存在
	_, err = driver.Get(ctx, id)
	if err != nil {
		t.Errorf("Get failed: %v", err)
	}
}

func TestFileDriverFinish(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}
	defer driver.Close()

	id := "finish-test.log"

	// 写入数据
	err = driver.Append(ctx, id, []byte("test data\n"), 0)
	if err != nil {
		t.Fatalf("Append failed: %v", err)
	}

	// 验证文件句柄已被缓存
	if _, ok := driver.files.Load(id); !ok {
		t.Error("Expected file handle to be cached")
	}

	// 调用 Finish 释放句柄
	err = driver.Finish(ctx, id)
	if err != nil {
		t.Fatalf("Finish failed: %v", err)
	}

	// 验证文件句柄已被释放
	if _, ok := driver.files.Load(id); ok {
		t.Error("Expected file handle to be released after Finish")
	}

	// 可以继续追加（会重新打开文件）
	err = driver.Append(ctx, id, []byte("more data\n"), 0)
	if err != nil {
		t.Fatalf("Append after Finish failed: %v", err)
	}
}

func TestFileDriverFinishNonExistent(t *testing.T) {
	ctx := context.Background()
	tmpDir := t.TempDir()

	driver, err := NewFileDriver(tmpDir)
	if err != nil {
		t.Fatalf("NewFileDriver failed: %v", err)
	}
	defer driver.Close()

	// 对不存在的文件调用 Finish 应该不报错
	err = driver.Finish(ctx, "non-existent.log")
	if err != nil {
		t.Errorf("Finish on non-existent file should not error, got %v", err)
	}
}
