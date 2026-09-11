// Package buildinfo 保存由构建过程注入的发布元数据。
package buildinfo

import (
	"fmt"
	"runtime/debug"
)

var (
	version       = "dev"
	commit        = "none"
	buildTime     = "unknown"
	archiveCommit = "$Format:%H$"
	archiveTime   = "$Format:%cI$"
)

// String 返回适合命令行显示的版本信息。
func String() string {
	resolvedVersion, resolvedCommit, resolvedBuildTime := resolve(version, commit, buildTime, archiveCommit, archiveTime, runtimeBuildInfo())
	return fmt.Sprintf("%s (commit %s, built %s)", resolvedVersion, resolvedCommit, resolvedBuildTime)
}

func runtimeBuildInfo() *debug.BuildInfo {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return nil
	}
	return info
}

func resolve(version, commit, buildTime, archiveCommit, archiveTime string, info *debug.BuildInfo) (string, string, string) {
	if info == nil {
		return resolveArchiveMetadata(version, commit, buildTime, archiveCommit, archiveTime)
	}
	if version == "dev" && info.Main.Version != "" && info.Main.Version != "(devel)" {
		version = info.Main.Version
	}
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			if commit == "none" && setting.Value != "" {
				commit = setting.Value
			}
		case "vcs.time":
			if buildTime == "unknown" && setting.Value != "" {
				buildTime = setting.Value
			}
		}
	}
	return resolveArchiveMetadata(version, commit, buildTime, archiveCommit, archiveTime)
}

func resolveArchiveMetadata(version, commit, buildTime, archiveCommit, archiveTime string) (string, string, string) {
	if commit == "none" && isArchiveValue(archiveCommit) {
		commit = archiveCommit
	}
	if buildTime == "unknown" && isArchiveValue(archiveTime) {
		buildTime = archiveTime
	}
	return version, commit, buildTime
}

func isArchiveValue(value string) bool {
	return value != "" && value[0] != '$'
}
