# Netkit

Netkit is a root-only Linux CLI for controlled Mihomo and EasyTier operations.

设计与命令契约见 [docs/DESIGN.md](docs/DESIGN.md)，开发进度见 [docs/DEVELOPMENT.md](docs/DEVELOPMENT.md)，Debian 12 安装与验收见 [docs/debian12.md](docs/debian12.md)。

```bash
go run ./cmd/netkit status
go run ./cmd/netkit links verify
go run ./cmd/netkit mihomo validate
go run ./cmd/netkit doctor
```

All commands must run as root. Production assets are expected in `/root/netkit`.

构建 Linux 发布包：

```bash
make release VERSION=v0.1.0
```
