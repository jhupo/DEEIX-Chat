package db

import (
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestProductionGORMLoggerIgnoresRecordNotFound(t *testing.T) {
	if output := captureProductionGORMTrace(t, gorm.ErrRecordNotFound); output != "" {
		t.Fatalf("expected record not found to be ignored, got %q", output)
	}

	output := captureProductionGORMTrace(t, errors.New("connection failed"))
	if !strings.Contains(output, "connection failed") {
		t.Fatalf("expected non-not-found error to be logged, got %q", output)
	}
}

func TestProductionGORMLoggerFiltersQueryParameters(t *testing.T) {
	logger := productionGORMLogger()
	filter, ok := logger.(gorm.ParamsFilter)
	if !ok {
		t.Fatal("production GORM logger does not filter query parameters")
	}
	sql, params := filter.ParamsFilter(context.Background(), "SELECT * FROM messages WHERE content = ?", "private conversation")
	if sql != "SELECT * FROM messages WHERE content = ?" || len(params) != 0 {
		t.Fatalf("filtered query = %q, %#v", sql, params)
	}
}

func captureProductionGORMTrace(t *testing.T, traceErr error) string {
	t.Helper()

	originalStdout := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatalf("create stdout pipe: %v", err)
	}
	os.Stdout = writer
	logger := productionGORMLogger()
	os.Stdout = originalStdout

	logger.Trace(context.Background(), time.Now(), func() (string, int64) {
		return "SELECT 1", 0
	}, traceErr)

	if err := writer.Close(); err != nil {
		t.Fatalf("close stdout pipe writer: %v", err)
	}
	output, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("read stdout pipe: %v", err)
	}
	return string(output)
}
