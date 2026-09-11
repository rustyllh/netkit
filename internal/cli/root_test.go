package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/rustyllh/netkit/internal/app"
	"github.com/rustyllh/netkit/internal/config"
	"github.com/rustyllh/netkit/internal/linux"
	"github.com/spf13/cobra"
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

type contextRecordingRunner struct{ hasDeadline bool }

func (r *contextRecordingRunner) Run(ctx context.Context, _ string, _ ...string) (linux.Result, error) {
	_, r.hasDeadline = ctx.Deadline()
	return linux.Result{}, nil
}

func TestLogsCommandTimeout(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantDeadline bool
	}{
		{
			name:         "普通日志使用超时",
			wantDeadline: true,
		},
		{
			name:         "持续日志不使用超时",
			args:         []string{"-f"},
			wantDeadline: false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &contextRecordingRunner{}
			cfg, err := config.Load(t.TempDir(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			newApp := func() (app.Application, error) {
				return app.New(cfg, runner), nil
			}
			render := func(*cobra.Command, app.Result) error { return nil }
			timeout := time.Nanosecond
			command := newLogsCommand("mihomo", newApp, render, &timeout)
			command.SetArgs(test.args)
			if err := command.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if runner.hasDeadline != test.wantDeadline {
				t.Fatalf("上下文 deadline = %v，期望 %v", runner.hasDeadline, test.wantDeadline)
			}
		})
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
