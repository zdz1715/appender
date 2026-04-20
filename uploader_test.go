package appender

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

// mockDriver 是用于测试的模拟 Driver
type mockDriver struct {
	data    []byte
	deleted bool
	offsets []int64
}

func (m *mockDriver) Get(ctx context.Context, id string) (*Metadata, error) {
	return &Metadata{Path: id}, nil
}

func (m *mockDriver) GetContent(ctx context.Context, id string) (io.ReadCloser, error) {
	r := bytes.NewReader(m.data)
	return io.NopCloser(r), nil
}

func (m *mockDriver) Delete(ctx context.Context, id string) error {
	m.deleted = true
	m.data = nil
	m.offsets = nil
	return nil
}

func (m *mockDriver) Append(ctx context.Context, id string, data []byte, offset int64) error {
	m.data = append(m.data, data...)
	m.offsets = append(m.offsets, offset)
	return nil
}

func TestStreamUploader(t *testing.T) {
	ctx := context.Background()

	t.Run("Basic upload", func(t *testing.T) {
		mock := &mockDriver{}
		input := "Line 1\nLine 2\nLine 3\n"
		reader := strings.NewReader(input)

		uploader := NewStreamUploader(
			reader,
			mock,
			WithUploadChunkSize(10),
			WithInterval(100*time.Millisecond),
		)

		go func() {
			if err := uploader.Run(ctx, "test-basic"); err != nil {
				t.Error(err)
			}
		}()

		<-uploader.Done() // Wait for upload to complete

		if string(mock.data) != input {
			t.Errorf("Expected uploaded data %q, got %q", input, string(mock.data))
		}
	})

	t.Run("Upload with description", func(t *testing.T) {
		mock := &mockDriver{}
		input := "Data\n"
		reader := strings.NewReader(input)
		desc := []byte("# Header\n")

		uploader := NewStreamUploader(
			reader,
			mock,
			WithDesc(desc),
			WithUploadChunkSize(10),
		)

		go func() {
			if err := uploader.Run(ctx, "test-basic"); err != nil {
				t.Error(err)
			}
		}()

		<-uploader.Done() // Wait for upload to complete

		// First chunk should be description
		if len(mock.data) == 0 {
			t.Fatal("No data uploaded")
		}

		if !bytes.HasPrefix(mock.data, desc) {
			t.Errorf("Expected first chunk to be description %q, got %q", desc, mock.data)
		}
	})

	t.Run("EOF handling", func(t *testing.T) {
		mock := &mockDriver{}

		// Create a reader that produces data slowly
		reader, writer := io.Pipe()

		uploader := NewStreamUploader(
			reader,
			mock,
			WithUploadChunkSize(5),
			WithInterval(50*time.Millisecond),
		)

		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()

		// Write some data then close to signal EOF
		go func() {
			writer.Write([]byte("Line 1\n"))
			time.Sleep(100 * time.Millisecond)
			writer.Write([]byte("Line 2\n"))
			time.Sleep(100 * time.Millisecond)
			writer.Close() // Close to signal EOF
		}()

		go uploader.Run(ctx, "test-eof")
		<-uploader.Done() // Wait for upload to complete

		// Verify data was uploaded
		if len(mock.data) == 0 {
			t.Error("Expected some data to be uploaded")
		}
	})

	t.Run("Context cancellation", func(t *testing.T) {
		mock := &mockDriver{}
		reader := strings.NewReader("Data\n")

		uploader := NewStreamUploader(
			reader,
			mock,
			WithUploadChunkSize(10),
		)

		ctx, cancel := context.WithCancel(context.Background())
		cancel()

		err := uploader.Run(ctx, "test-cancel")
		if err != context.Canceled {
			t.Errorf("Expected context.Canceled error, got %v", err)
		}
	})
}

func TestStreamUploaderOptions(t *testing.T) {
	mock := &mockDriver{}
	reader := strings.NewReader("test")

	t.Run("WithUploadChunkSize", func(t *testing.T) {
		uploader := NewStreamUploader(
			reader,
			mock,
			WithUploadChunkSize(2048),
		)

		if uploader.opts.uploadChunkSize != 2048 {
			t.Errorf("Expected uploadChunkSize 2048, got %d", uploader.opts.uploadChunkSize)
		}
	})

	t.Run("WithReadBuffSize", func(t *testing.T) {
		uploader := NewStreamUploader(
			reader,
			mock,
			WithReadBuffSize(8192),
		)

		if uploader.opts.readBuffSize != 8192 {
			t.Errorf("Expected readBuffSize 8192, got %d", uploader.opts.readBuffSize)
		}
	})

	t.Run("WithInterval", func(t *testing.T) {
		interval := 1 * time.Second
		uploader := NewStreamUploader(
			reader,
			mock,
			WithInterval(interval),
		)

		if uploader.opts.interval != interval {
			t.Errorf("Expected interval %v, got %v", interval, uploader.opts.interval)
		}
	})
}

func TestStreamUploaderNilDriver(t *testing.T) {
	ctx := context.Background()
	input := "test\n"
	reader := strings.NewReader(input)

	// Uploader with nil driver should not panic
	uploader := NewStreamUploader(reader, nil)

	err := uploader.Run(ctx, "test-nil")
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	// Should complete without error even with nil driver
}

func TestStreamUploaderWithLinePrefix(t *testing.T) {
	ctx := context.Background()

	t.Run("Basic line prefix", func(t *testing.T) {
		mock := &mockDriver{}
		input := "Line 1\nLine 2\nLine 3\n"
		reader := strings.NewReader(input)

		uploader := NewStreamUploader(
			reader,
			mock,
			WithUploadChunkSize(100),
			WithLinePrefix(func(lineNum int64, readLen int64) []byte {
				return []byte(fmt.Sprintf("[%d] ", lineNum))
			}),
		)

		go func() {
			if err := uploader.Run(ctx, "test-prefix"); err != nil {
				t.Error(err)
			}
		}()

		<-uploader.Done() // Wait for upload to complete

		expected := "[1] Line 1\n[2] Line 2\n[3] Line 3\n"
		if string(mock.data) != expected {
			t.Errorf("Expected %q, got %q", expected, string(mock.data))
		}
	})

	t.Run("Prefix with read length", func(t *testing.T) {
		mock := &mockDriver{}
		input := "Hello\nWorld\n"
		reader := strings.NewReader(input)

		uploader := NewStreamUploader(
			reader,
			mock,
			WithUploadChunkSize(100),
			WithLinePrefix(func(lineNum int64, readLen int64) []byte {
				return []byte(fmt.Sprintf("L%d-%d|", lineNum, readLen))
			}),
		)

		go func() {
			if err := uploader.Run(ctx, "test-prefix-len"); err != nil {
				t.Error(err)
			}
		}()

		<-uploader.Done()

		expected := "L1-6|Hello\nL2-12|World\n"
		if string(mock.data) != expected {
			t.Errorf("Expected %q, got %q", expected, string(mock.data))
		}
	})

	t.Run("Empty prefix", func(t *testing.T) {
		mock := &mockDriver{}
		input := "Line 1\n"
		reader := strings.NewReader(input)

		uploader := NewStreamUploader(
			reader,
			mock,
			WithUploadChunkSize(100),
			WithLinePrefix(func(lineNum int64, readLen int64) []byte {
				return []byte("")
			}),
		)

		go func() {
			if err := uploader.Run(ctx, "test-empty-prefix"); err != nil {
				t.Error(err)
			}
		}()

		<-uploader.Done()

		expected := "Line 1\n"
		if string(mock.data) != expected {
			t.Errorf("Expected %q, got %q", expected, string(mock.data))
		}
	})

	t.Run("Prefix with description", func(t *testing.T) {
		mock := &mockDriver{}
		input := "Data\n"
		reader := strings.NewReader(input)
		desc := []byte("# Log\n")

		uploader := NewStreamUploader(
			reader,
			mock,
			WithDesc(desc),
			WithUploadChunkSize(100),
			WithLinePrefix(func(lineNum int64, readLen int64) []byte {
				return []byte(fmt.Sprintf("[%d] ", lineNum))
			}),
		)

		go func() {
			if err := uploader.Run(ctx, "test-desc-prefix"); err != nil {
				t.Error(err)
			}
		}()

		<-uploader.Done()

		expected := "# Log\n[1] Data\n"
		if string(mock.data) != expected {
			t.Errorf("Expected %q, got %q", expected, string(mock.data))
		}
	})

	t.Run("Multiple chunks with prefix", func(t *testing.T) {
		mock := &mockDriver{}
		input := "1\n2\n3\n4\n5\n6\n7\n8\n9\n10\n"
		reader := strings.NewReader(input)

		uploader := NewStreamUploader(
			reader,
			mock,
			WithUploadChunkSize(15), // Each chunk will contain ~1.5 lines
			WithLinePrefix(func(lineNum int64, readLen int64) []byte {
				return []byte(fmt.Sprintf("LINE%d ", lineNum))
			}),
		)

		go func() {
			if err := uploader.Run(ctx, "test-multi-chunk"); err != nil {
				t.Error(err)
			}
		}()

		<-uploader.Done()

		expected := "LINE1 1\nLINE2 2\nLINE3 3\nLINE4 4\nLINE5 5\nLINE6 6\nLINE7 7\nLINE8 8\nLINE9 9\nLINE10 10\n"
		if string(mock.data) != expected {
			t.Errorf("Expected %q, got %q", expected, string(mock.data))
		}
	})
}
