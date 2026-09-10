package linux

import (
	"context"
	"testing"
	"time"
)

func TestOSRunner(t *testing.T) {
	runner := OSRunner{}
	tests := []struct {
		name      string
		ctx       context.Context
		program   string
		args      []string
		wantError bool
		wantCode  int
	}{
		{"非零退出码", context.Background(), "sh", []string{"-c", "exit 7"}, false, 7},
		{"不存在的命令", context.Background(), "netkit-command-does-not-exist", nil, true, 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			result, err := runner.Run(test.ctx, test.program, test.args...)
			if (err != nil) != test.wantError {
				t.Fatalf("Run() error = %v, wantError %v", err, test.wantError)
			}
			if result.ExitCode != test.wantCode {
				t.Fatalf("Run() ExitCode = %d, want %d", result.ExitCode, test.wantCode)
			}
		})
	}
}

func TestOSRunnerTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	result, err := OSRunner{}.Run(ctx, "sleep", "1")
	if err != nil {
		t.Fatalf("Run() error = %v", err)
	}
	if result.ExitCode == 0 {
		t.Fatal("超时命令的退出码为 0")
	}
}
