package appender

import (
	"bytes"
	"context"
	"os"
	"testing"
)

func TestConsoleDriver(t *testing.T) {
	ctx := context.Background()
	driver := NewConsoleDriver(os.Stderr)

	t.Run("Append", func(t *testing.T) {
		err := driver.Append(ctx, "test-id", []byte("Hello, World!\n"), 0)
		if err != nil {
			t.Fatalf("Append failed: %v", err)
		}
	})

	t.Run("Get", func(t *testing.T) {
		entry, _ := driver.Get(ctx, "test-id")
		if entry == nil {
			t.Fatal("Entry should not be nil")
		}
	})

	t.Run("Delete", func(t *testing.T) {
		err := driver.Delete(ctx, "test-id")
		if err != nil {
			t.Fatalf("Delete failed: %v", err)
		}
	})
}

func TestConsoleDriverOutput(t *testing.T) {
	ctx := context.Background()
	var buf bytes.Buffer
	driver := NewConsoleDriver(&buf)

	// Append some data
	data1 := []byte("First line\n")
	data2 := []byte("Second line\n")

	err := driver.Append(ctx, "test-output", data1, 0)
	if err != nil {
		t.Fatalf("First Append failed: %v", err)
	}

	err = driver.Append(ctx, "test-output", data2, int64(len(data1)))
	if err != nil {
		t.Fatalf("Second Append failed: %v", err)
	}

	// Check output
	output := buf.String()
	expected := "First line\nSecond line\n"
	if output != expected {
		t.Errorf("Expected output %q, got %q", expected, output)
	}
}
