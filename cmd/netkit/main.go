// Command netkit 提供对 Mihomo 和 EasyTier 的受控运维能力。
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/rustyllh/netkit/internal/buildinfo"
	"github.com/rustyllh/netkit/internal/cli"
)

func main() {
	command := cli.NewRootCommand()
	command.Version = buildinfo.String()
	if err := command.ExecuteContext(context.Background()); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitCode(err))
	}
}
