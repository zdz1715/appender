package appender

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestFileFollower(t *testing.T) {
	ctx := context.Background()

	t.Run("Basic file following", func(t *testing.T) {
		mock := &mockDriver{}

		// Create a temporary file
		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "test.log")

		// Write initial content
		err := os.WriteFile(tmpFile, []byte("Initial line\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		follower := NewFileFollower(
			tmpFile,
			mock,
			WithUploadChunkSize(10),
			WithInterval(100*time.Millisecond),
		)

		stopCh := make(chan struct{})

		// Write more data after a short delay
		go func() {
			time.Sleep(150 * time.Millisecond)
			f, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				t.Errorf("Failed to open file for append: %v", err)
				return
			}
			defer f.Close()

			f.WriteString("Second line\n")
			f.WriteString("Third line\n")

			time.Sleep(100 * time.Millisecond)
			close(stopCh)
		}()

		if err = follower.Run(ctx, "test-basic", stopCh); err != nil {
			t.Error(err)
		}

		// Verify data was uploaded
		if len(mock.data) == 0 {
			t.Fatal("No data uploaded")
		}

		expected := "Initial line\nSecond line\nThird line\n"
		if string(mock.data) != expected {
			t.Errorf("Expected uploaded data %q, got %q", expected, string(mock.data))
		}
	})

	t.Run("RunFrom with offset", func(t *testing.T) {
		mock := &mockDriver{}

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "offset.log")

		// Write initial content
		content := "Line 1\nLine 2\nLine 3\nLine 4\n"
		err := os.WriteFile(tmpFile, []byte(content), 0644)
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		follower := NewFileFollower(
			tmpFile,
			mock,
			WithUploadChunkSize(20),
		)

		// Start from offset 7 (after "Line 1\n")
		stopCh := make(chan struct{})
		close(stopCh) // Stop immediately

		err = follower.RunFrom(ctx, "test-offset", 7, stopCh)
		if err != nil {
			t.Fatalf("RunFrom failed: %v", err)
		}

		// Should only have data from offset 7 onwards
		if len(mock.data) == 0 {
			t.Fatal("No data uploaded")
		}

		expected := "Line 2\nLine 3\nLine 4\n"
		if string(mock.data) != expected {
			t.Errorf("Expected uploaded data from offset %q, got %q", expected, string(mock.data))
		}
	})

	t.Run("RunFrom with dynamic file growth", func(t *testing.T) {
		mock := &mockDriver{}

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "dynamic.log")

		// Write initial content
		initialContent := "Initial Line 1\nInitial Line 2\n"
		err := os.WriteFile(tmpFile, []byte(initialContent), 0644)
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		// Calculate offset to start from middle of initial content
		// "Initial Line 1\n" is 15 bytes, so we start from there
		startOffset := int64(len("Initial Line 1\n"))

		follower := NewFileFollower(
			tmpFile,
			mock,
			WithUploadChunkSize(20),
			WithInterval(100*time.Millisecond),
		)

		stopCh := make(chan struct{})

		// Append more content after a delay
		go func() {
			time.Sleep(150 * time.Millisecond)
			f, err := os.OpenFile(tmpFile, os.O_APPEND|os.O_WRONLY, 0644)
			if err != nil {
				t.Errorf("Failed to open file for append: %v", err)
				return
			}
			defer f.Close()

			f.WriteString("Appended Line 3\n")
			f.WriteString("Appended Line 4\n")

			time.Sleep(100 * time.Millisecond)
			close(stopCh)
		}()

		// Start from offset
		err = follower.RunFrom(ctx, "test-dynamic", startOffset, stopCh)
		if err != nil {
			t.Errorf("RunFrom failed: %v", err)
		}

		// Verify data was uploaded
		if len(mock.data) == 0 {
			t.Fatal("No data uploaded")
		}

		// Should have content from offset onwards, including appended data
		expected := "Initial Line 2\nAppended Line 3\nAppended Line 4\n"
		if string(mock.data) != expected {
			t.Errorf("Expected uploaded data %q, got %q", expected, string(mock.data))
		}
	})

	t.Run("File truncation handling", func(t *testing.T) {
		mock := &mockDriver{}

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "truncate.log")

		// Write initial content
		err := os.WriteFile(tmpFile, []byte("Long initial content\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		follower := NewFileFollower(
			tmpFile,
			mock,
			WithUploadChunkSize(10),
			WithInterval(50*time.Millisecond),
		)

		stopCh := make(chan struct{})

		// Truncate file and write new content
		go func() {
			time.Sleep(100 * time.Millisecond)
			err := os.WriteFile(tmpFile, []byte("New content\n"), 0644)
			if err != nil {
				t.Errorf("Failed to truncate file: %v", err)
				return
			}
			time.Sleep(100 * time.Millisecond)
			close(stopCh)
		}()

		err = follower.Run(ctx, "test-truncate", stopCh)
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}

		// Should handle truncation without error
		if len(mock.data) == 0 {
			t.Error("Expected some data to be uploaded")
		}
	})

	t.Run("Non-existent file", func(t *testing.T) {
		mock := &mockDriver{}

		follower := NewFileFollower(
			"/non/existent/file.log",
			mock,
		)

		stopCh := make(chan struct{})
		close(stopCh)

		err := follower.Run(ctx, "test-nonexistent", stopCh)
		if err == nil {
			t.Error("Expected error for non-existent file")
		}
	})

	t.Run("File with description", func(t *testing.T) {
		mock := &mockDriver{}

		tmpDir := t.TempDir()
		tmpFile := filepath.Join(tmpDir, "desc.log")

		err := os.WriteFile(tmpFile, []byte("Data\n"), 0644)
		if err != nil {
			t.Fatalf("Failed to create temp file: %v", err)
		}

		desc := []byte("# Log File Header\n")

		follower := NewFileFollower(
			tmpFile,
			mock,
			WithDesc(desc),
			WithUploadChunkSize(10),
		)

		stopCh := make(chan struct{})
		close(stopCh)

		err = follower.Run(ctx, "test-desc", stopCh)
		if err != nil {
			t.Fatalf("Run failed: %v", err)
		}

		// First chunk should be description
		if len(mock.data) == 0 {
			t.Fatal("No data uploaded")
		}

		if !bytes.HasPrefix(mock.data, []byte("# Log File Header\n")) {
			t.Errorf("Expected first chunk to start with description, got %q", string(mock.data))
		}
	})
}

func TestFileFollowerOptions(t *testing.T) {
	mock := &mockDriver{}
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "options.log")

	os.WriteFile(tmpFile, []byte("test"), 0644)

	t.Run("WithUploadChunkSize", func(t *testing.T) {
		follower := NewFileFollower(
			tmpFile,
			mock,
			WithUploadChunkSize(2048),
		)

		if follower.opts.uploadChunkSize != 2048 {
			t.Errorf("Expected uploadChunkSize 2048, got %d", follower.opts.uploadChunkSize)
		}
	})

	t.Run("WithReadBuffSize", func(t *testing.T) {
		follower := NewFileFollower(
			tmpFile,
			mock,
			WithReadBuffSize(8192),
		)

		if follower.opts.readBuffSize != 8192 {
			t.Errorf("Expected readBuffSize 8192, got %d", follower.opts.readBuffSize)
		}
	})

	t.Run("WithInterval", func(t *testing.T) {
		interval := 1 * time.Second
		follower := NewFileFollower(
			tmpFile,
			mock,
			WithInterval(interval),
		)

		if follower.opts.interval != interval {
			t.Errorf("Expected interval %v, got %v", interval, follower.opts.interval)
		}
	})
}

func TestFileFollowerReset(t *testing.T) {
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "reset.log")

	// Create a file with some content
	content := "Line 1\nLine 2\nLine 3\n"
	err := os.WriteFile(tmpFile, []byte(content), 0644)
	if err != nil {
		t.Fatalf("Failed to create temp file: %v", err)
	}

	mock := &mockDriver{}
	follower := NewFileFollower(tmpFile, mock)

	// Test reset with offset
	err = follower.reset(7) // Skip "Line 1\n"
	if err != nil {
		t.Fatalf("reset failed: %v", err)
	}

	if follower.readLen != 7 {
		t.Errorf("Expected readLen 7, got %d", follower.readLen)
	}
}
