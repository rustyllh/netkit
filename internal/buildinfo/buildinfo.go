// Package buildinfo 保存由构建过程注入的发布元数据。
package buildinfo

import "fmt"

var (
	version   = "dev"
	commit    = "none"
	buildTime = "unknown"
)

// String 返回适合命令行显示的版本信息。
func String() string {
	return fmt.Sprintf("netkit %s (commit %s, built %s)", version, commit, buildTime)
}
