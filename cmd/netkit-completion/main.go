// Command netkit-completion 为发布包生成 Shell 补全文件。
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rustyllh/netkit/internal/cli"
)

func main() {
	outputDir := flag.String("output", "", "补全文件输出目录")
	flag.Parse()
	if *outputDir == "" {
		fmt.Fprintln(os.Stderr, "缺少 --output")
		os.Exit(1)
	}
	if err := generate(*outputDir); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate(outputDir string) error {
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		return fmt.Errorf("创建补全目录: %w", err)
	}
	command := cli.NewRootCommand()
	if err := command.GenBashCompletionFile(filepath.Join(outputDir, "netkit.bash")); err != nil {
		return fmt.Errorf("生成 Bash 补全: %w", err)
	}
	if err := command.GenZshCompletionFile(filepath.Join(outputDir, "_netkit")); err != nil {
		return fmt.Errorf("生成 Zsh 补全: %w", err)
	}
	if err := command.GenFishCompletionFile(filepath.Join(outputDir, "netkit.fish"), true); err != nil {
		return fmt.Errorf("生成 Fish 补全: %w", err)
	}
	return nil
}
