package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/rustyllh/netkit/internal/app"
	"github.com/rustyllh/netkit/internal/config"
	"github.com/rustyllh/netkit/internal/linux"
	"github.com/spf13/cobra"
)

var errRootRequired = errors.New("netkit must run as root")

type resultOperation func(context.Context, app.Application) app.Result
type streamOperation func(context.Context, app.Application, io.Writer, io.Writer) error

// commandRuntime 集中处理命令执行所需的权限、配置、超时及结果渲染。
type commandRuntime struct {
	newApp     func() (app.Application, error)
	timeout    func() time.Duration
	jsonOutput func() bool
	euid       func() int
}

func newCommandRuntime(rootDir *string, timeout *time.Duration, jsonOutput *bool) commandRuntime {
	return commandRuntime{
		newApp: func() (app.Application, error) {
			cfg, err := config.Load(*rootDir, *timeout)
			if err != nil {
				return app.Application{}, err
			}
			return app.New(cfg, linux.OSRunner{}), nil
		},
		timeout:    func() time.Duration { return *timeout },
		jsonOutput: func() bool { return *jsonOutput },
		euid:       os.Geteuid,
	}
}

// run 执行会在超时内完成、并返回结构化结果的命令。
func (r commandRuntime) run(cmd *cobra.Command, operation resultOperation) error {
	application, err := r.application()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(cmd.Context(), r.timeout())
	defer cancel()
	return writeResult(cmd.OutOrStdout(), operation(ctx, application), r.jsonOutput())
}

// stream 执行不应施加命令超时的持续输出命令，例如日志跟随。
func (r commandRuntime) stream(cmd *cobra.Command, operation streamOperation) error {
	application, err := r.application()
	if err != nil {
		return err
	}
	return operation(cmd.Context(), application, cmd.OutOrStdout(), cmd.ErrOrStderr())
}

func (r commandRuntime) application() (app.Application, error) {
	if r.euid() != 0 {
		return app.Application{}, errRootRequired
	}
	return r.newApp()
}

func newResultCommand(
	runtime commandRuntime,
	use string,
	short string,
	args cobra.PositionalArgs,
	operation resultOperation,
) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: short,
		Args:  args,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runtime.run(cmd, operation)
		},
	}
}

func writeResult(writer io.Writer, result app.Result, jsonOutput bool) error {
	if jsonOutput {
		return json.NewEncoder(writer).Encode(result)
	}
	for _, check := range result.Checks {
		state := "ok"
		if !check.OK {
			state = check.Severity
		}
		if _, err := fmt.Fprintf(writer, "%-7s %-24s %s\n", state, check.Name, check.Detail); err != nil {
			return err
		}
	}
	if !result.OK {
		return errors.New("one or more checks failed")
	}
	return nil
}
