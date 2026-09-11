package systemd

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/rustyllh/netkit/internal/linux"
)

type fakeRunner struct {
	result linux.Result
	err    error
}

func (f fakeRunner) Run(context.Context, string, ...string) (linux.Result, error) {
	return f.result, f.err
}

func TestRestartRejectsNonZeroExitCode(t *testing.T) {
	client := NewClient(fakeRunner{result: linux.Result{ExitCode: 1, Stderr: "permission denied"}})
	if err := client.Restart(context.Background(), "mihomo.service"); err == nil {
		t.Fatal("Restart() 在非零退出码时应失败")
	}
}

func TestLogArgs(t *testing.T) {
	tests := []struct {
		name    string
		lines   int
		since   time.Duration
		follow  bool
		want    []string
		wantErr bool
	}{
		{
			name:  "默认按行数读取",
			lines: 100,
			want:  []string{"--unit", "mihomo.service", "--no-pager", "--output", "short-iso", "--lines", "100"},
		},
		{
			name:   "按时间过滤并持续跟随",
			lines:  300,
			since:  time.Hour,
			follow: true,
			want:   []string{"--unit", "mihomo.service", "--no-pager", "--output", "short-iso", "--lines", "300", "--since", "1h0m0s ago", "--follow"},
		},
		{
			name:    "拒绝非正数行数",
			lines:   0,
			wantErr: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := logArgs("mihomo.service", test.lines, test.since, test.follow)
			if (err != nil) != test.wantErr {
				t.Fatalf("logArgs() error = %v, wantErr %v", err, test.wantErr)
			}
			if !slices.Equal(got, test.want) {
				t.Fatalf("logArgs() = %q, want %q", got, test.want)
			}
		})
	}
}
