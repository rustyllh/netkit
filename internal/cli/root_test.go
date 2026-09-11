package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rustyllh/netkit/internal/app"
)

func TestRootCommandDoesNotExposeCompletion(t *testing.T) {
	command := NewRootCommand()
	var output bytes.Buffer
	command.SetOut(&output)
	command.SetArgs([]string{"--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "completion") {
		t.Fatalf("帮助信息不应包含 completion：%s", output.String())
	}
}

func TestWriteResultJSONContract(t *testing.T) {
	value := app.Result{
		OK:        true,
		Operation: "status",
		Checks:    []app.Check{{Name: "mihomo service", OK: true, Severity: "error", Detail: "active"}},
		Warnings:  []string{},
	}
	var output bytes.Buffer
	if err := writeResult(&output, value, true); err != nil {
		t.Fatalf("writeResult() error = %v", err)
	}
	var decoded map[string]json.RawMessage
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("JSON 输出无法解析: %v", err)
	}
	for _, field := range []string{"ok", "operation", "checks"} {
		if _, exists := decoded[field]; !exists {
			t.Errorf("JSON 输出缺少 %q 字段", field)
		}
	}
}
