# Netkit

Netkit 是一个仅面向 Linux root 用户的网络基础服务运维 CLI。它管理 Mihomo 与 EasyTier 的配置校验、状态诊断、日志、快照、受控重启和回滚；Docker Compose 及业务资产不在管理范围内。

生产部署资产固定在 `/root/netkit`，业务目录必须位于该目录之外。Netkit 使用 systemd 执行服务启停，不替代 systemd。

## 功能

| 范围 | 能力 |
| --- | --- |
| 全局诊断 | 服务状态、软链接验证、Mihomo 端口、EasyTier 网卡与 Docker 代理关联检查 |
| Mihomo | 配置校验、journald 日志、健康检查、快照、受控 apply / rollback |
| EasyTier | TOML 校验、peer 查询与 ping、journald 日志、快照、受控 apply / rollback |
| 备份 | 创建、列出、校验受限配置快照；不会包含业务数据或 provider 缓存 |
| 审计 | 记录配置变更与校验事件，并脱敏 token、secret、API key |

`--dry-run` 只展示计划，不修改服务或文件；不带该参数时会直接执行对应操作。

## 安装

要求：Linux、root 权限。Mihomo 与 EasyTier 二进制需已按服务器约定安装。

推荐从 GitHub Release 安装。脚本会识别 amd64/arm64、校验 SHA256，将 `netkit` 安装到 `/usr/local/bin`，并在系统支持时安装 Bash、Zsh、Fish 补全：

```bash
curl -fsSL https://raw.githubusercontent.com/rustyllh/netkit/main/install.sh | sh
netkit --version
```

安装指定版本：

```bash
curl -fsSL https://raw.githubusercontent.com/rustyllh/netkit/main/install.sh | NETKIT_VERSION=v0.1.3 sh
```

默认安装目录为 `/usr/local/bin`；可通过 `NETKIT_INSTALL_DIR` 覆盖。非 root 用户需要 `sudo`。若已安装 `bash-completion`，脚本会配置 `/root/.bashrc`；重新登录或执行 `source /root/.bashrc` 后生效。

也可使用 Go 从源码编译安装。此方式要求 Go 1.26+，且版本信息不一定包含 Git commit 与构建时间：

```bash
GOBIN=/usr/local/bin go install github.com/rustyllh/netkit/cmd/netkit@latest
netkit --version
```

私有仓库需要先为 Git 配置访问 GitHub 的凭据，并设置：

```bash
go env -w GOPRIVATE=github.com/rustyllh/*
```

安装后的首次检查：

```bash
netkit links verify
netkit doctor
netkit backup create
```

Debian 12 的目录约定、系统依赖和目标机验收步骤见 [docs/debian12.md](docs/debian12.md)。

## 常用命令

```bash
# 总览和诊断
netkit status
netkit doctor
netkit links verify

# Mihomo
netkit mihomo validate
netkit mihomo logs --since 1h
netkit mihomo apply --dry-run
netkit mihomo apply
netkit mihomo rollback <SNAPSHOT_ID>

# EasyTier
netkit easytier status
netkit easytier peers
netkit easytier ping 10.126.126.3
netkit easytier apply --dry-run
netkit easytier apply

# 配置快照
netkit backup create
netkit backup list
netkit backup verify <SNAPSHOT_ID>
```

## 本地开发与构建

```bash
make fmt-check
make vet
make test
make build
```

`make release` 生成 Linux amd64/arm64 二进制及校验和。推送 `v*` tag 时，GitHub Actions 会将这些文件发布到 GitHub Release。
