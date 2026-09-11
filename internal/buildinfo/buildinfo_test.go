package buildinfo

import (
	"runtime/debug"
	"testing"
)

func TestResolve(t *testing.T) {
	tests := []struct {
		name          string
		version       string
		commit        string
		buildTime     string
		archiveCommit string
		archiveTime   string
		info          *debug.BuildInfo
		wantVersion   string
		wantCommit    string
		wantBuildTime string
	}{
		{
			name:          "构建参数优先",
			version:       "v1.2.3",
			commit:        "release-commit",
			buildTime:     "2026-09-11T00:00:00Z",
			archiveCommit: "archive-commit",
			archiveTime:   "2026-01-01T00:00:00Z",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "v9.9.9"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "runtime-commit"},
					{Key: "vcs.time", Value: "2026-01-01T00:00:00Z"},
				},
			},
			wantVersion:   "v1.2.3",
			wantCommit:    "release-commit",
			wantBuildTime: "2026-09-11T00:00:00Z",
		},
		{
			name:          "回退到模块与 VCS 元数据",
			version:       "dev",
			commit:        "none",
			buildTime:     "unknown",
			archiveCommit: "archive-commit",
			archiveTime:   "2026-09-10T00:00:00Z",
			info: &debug.BuildInfo{
				Main: debug.Module{Version: "v1.2.3"},
				Settings: []debug.BuildSetting{
					{Key: "vcs.revision", Value: "runtime-commit"},
					{Key: "vcs.time", Value: "2026-09-11T00:00:00Z"},
				},
			},
			wantVersion:   "v1.2.3",
			wantCommit:    "runtime-commit",
			wantBuildTime: "2026-09-11T00:00:00Z",
		},
		{
			name:          "回退到归档元数据",
			version:       "dev",
			commit:        "none",
			buildTime:     "unknown",
			archiveCommit: "archive-commit",
			archiveTime:   "2026-09-11T00:00:00Z",
			wantVersion:   "dev",
			wantCommit:    "archive-commit",
			wantBuildTime: "2026-09-11T00:00:00Z",
		},
		{
			name:          "缺少构建信息时保留默认值",
			version:       "dev",
			commit:        "none",
			buildTime:     "unknown",
			wantVersion:   "dev",
			wantCommit:    "none",
			wantBuildTime: "unknown",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			gotVersion, gotCommit, gotBuildTime := resolve(test.version, test.commit, test.buildTime, test.archiveCommit, test.archiveTime, test.info)
			if gotVersion != test.wantVersion || gotCommit != test.wantCommit || gotBuildTime != test.wantBuildTime {
				t.Fatalf("resolve() = (%q, %q, %q)，期望 (%q, %q, %q)", gotVersion, gotCommit, gotBuildTime, test.wantVersion, test.wantCommit, test.wantBuildTime)
			}
		})
	}
}
