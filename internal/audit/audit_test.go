package audit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppendRedactsSensitiveDetails(t *testing.T) {
	root := t.TempDir()
	err := Append(context.Background(), root, Event{Operation: "backup create", Target: "mihomo", Result: "success", Details: map[string]string{"url": "https://example.test/?token=secret-value&api_key=key-value"}})
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, ".netkit", "audit.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if strings.Contains(text, "secret-value") || strings.Contains(text, "key-value") {
		t.Fatalf("审计日志泄露敏感信息: %s", text)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("日志权限 = %o, want 600", info.Mode().Perm())
	}
}
