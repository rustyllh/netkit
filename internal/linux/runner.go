// Package linux 提供 Linux 主机操作所需的小型适配器。
package linux

import (
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Result 保存外部命令完成后的输出和退出状态。
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Runner 在不经由 shell 的情况下执行进程。
type Runner interface {
	Run(context.Context, string, ...string) (Result, error)
}

// OSRunner 执行主机上的二进制文件。
type OSRunner struct{}

// Run 以 args 执行 program，并捕获标准输出和标准错误。
func (OSRunner) Run(ctx context.Context, program string, args ...string) (Result, error) {
	command := exec.CommandContext(ctx, program, args...)
	output, err := command.Output()
	result := Result{Stdout: strings.TrimSpace(string(output))}
	if err == nil {
		return result, nil
	}

	if exitErr, ok := err.(*exec.ExitError); ok {
		result.Stderr = strings.TrimSpace(string(exitErr.Stderr))
		result.ExitCode = exitErr.ExitCode()
		return result, nil
	}
	return result, fmt.Errorf("run %s: %w", program, err)
}
