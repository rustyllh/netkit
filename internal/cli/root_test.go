package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
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

func TestCommandRuntimeRejectsNonRoot(t *testing.T) {
	createdApplication := false
	runtime := commandRuntime{
		newApp: func() (app.Application, error) {
			createdApplication = true
			return app.Application{}, nil
		},
		euid: func() int { return 1000 },
	}
	err := runtime.run(&cobra.Command{}, func(context.Context, app.Application) app.Result {
		return app.Result{}
	})
	if !errors.Is(err, errRootRequired) {
		t.Fatalf("run() error = %v, 期望 root 权限错误", err)
	}
	if createdApplication {
		t.Fatal("非 root 用户不应加载应用配置")
	}
}

type contextRecordingRunner struct{ hasDeadline bool }

func (r *contextRecordingRunner) Run(ctx context.Context, _ string, _ ...string) (linux.Result, error) {
	_, r.hasDeadline = ctx.Deadline()
	return linux.Result{}, nil
}

func (r *contextRecordingRunner) Stream(
	ctx context.Context,
	stdout io.Writer,
	_ io.Writer,
	_ string,
	_ ...string,
) error {
	_, r.hasDeadline = ctx.Deadline()
	_, err := io.WriteString(stdout, "streamed log\n")
	return err
}

func TestLogsCommandTimeout(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		wantDeadline bool
		wantOutput   string
	}{
		{
			name:         "普通日志使用超时",
			wantDeadline: true,
			wantOutput:   "ok      mihomo logs              \n",
		},
		{
			name:         "持续日志不使用超时",
			args:         []string{"-f"},
			wantDeadline: false,
			wantOutput:   "streamed log\n",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			runner := &contextRecordingRunner{}
			cfg, err := config.Load(t.TempDir(), time.Second)
			if err != nil {
				t.Fatal(err)
			}
			timeout := time.Nanosecond
			runtime := commandRuntime{
				newApp: func() (app.Application, error) {
					return app.New(cfg, runner), nil
				},
				timeout:    func() time.Duration { return timeout },
				jsonOutput: func() bool { return false },
				euid:       func() int { return 0 },
			}
			command := newLogsCommand("mihomo", runtime)
			var output bytes.Buffer
			command.SetOut(&output)
			command.SetArgs(test.args)
			if err := command.ExecuteContext(context.Background()); err != nil {
				t.Fatal(err)
			}
			if runner.hasDeadline != test.wantDeadline {
				t.Fatalf("上下文 deadline = %v，期望 %v", runner.hasDeadline, test.wantDeadline)
			}
			if output.String() != test.wantOutput {
				t.Fatalf("命令输出 = %q，期望 %q", output.String(), test.wantOutput)
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

func TestWriteResultMultilineDetail(t *testing.T) {
	value := app.Result{
		OK:        true,
		Operation: "easytier status",
		Checks: []app.Check{{
			Name:     "easytier peers",
			OK:       true,
			Severity: "error",
			Detail:   "| ipv4 | hostname |\n| --- | --- |\n| 10.0.0.1 | node |",
		}},
		Warnings: []string{},
	}
	var output bytes.Buffer
	if err := writeResult(&output, value, false); err != nil {
		t.Fatal(err)
	}
	want := "ok      easytier peers          \n| ipv4 | hostname |\n| --- | --- |\n| 10.0.0.1 | node |\n"
	if output.String() != want {
		t.Fatalf("多行详情输出 = %q，期望 %q", output.String(), want)
	}
}
