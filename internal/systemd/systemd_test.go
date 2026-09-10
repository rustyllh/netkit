package systemd

import (
	"context"
	"testing"

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
