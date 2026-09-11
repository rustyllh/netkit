# Debian 12 安装与验收

## 安装前提

目标机必须以 root 运行，并已安装：Docker CE、Mihomo、EasyTier，以及 `curl`、`ca-certificates`、`iproute2`。Mihomo 与 EasyTier 二进制分别位于 `/usr/local/bin/mihomo`、`/usr/local/bin/easytier-core` 和 `/usr/local/bin/easytier-cli`。

部署资产只位于 `/root/netkit`。业务 Compose、OAuth、Keeper 数据不属于 Netkit 资产，也不得放入该目录。

## 安装二进制

在构建机执行：

```bash
make release VERSION=v0.1.0
scp dist/netkit_linux_amd64.tar.gz root@HOST:/tmp/
```

相同源码、`VERSION`、`COMMIT` 和 `BUILD_TIME` 参数会得到相同发布包；正式发布应显式传入三者。

在目标机执行：

```bash
tar -xzf /tmp/netkit_linux_amd64.tar.gz -C /tmp
install -m 0755 /tmp/netkit /usr/local/bin/netkit
netkit --version
```

若目标为 arm64，使用 `netkit_linux_arm64.tar.gz`。

## 资产与标准入口

```text
/etc/mihomo                        -> /root/netkit/mihomo/config
/etc/easytier                      -> /root/netkit/easytier/config
/etc/systemd/system/mihomo.service -> /root/netkit/services/mihomo.service
/etc/systemd/system/easytier.service -> /root/netkit/services/easytier.service
```

不要用 Netkit 覆盖已有 `/etc` 文件。先按迁移文档手动建立或核实链接，再执行：

```bash
netkit links verify
netkit doctor
```

## 首次验收

```bash
netkit status
netkit mihomo validate
netkit easytier validate
netkit easytier peers
netkit backup create
netkit backup list
```

若上述只读检查正常，再演练受控变更。先使用 `--dry-run` 确认计划：

```bash
netkit mihomo apply --dry-run
netkit easytier apply --dry-run
```

生产环境的 apply/rollback 演练应在维护窗口执行，并保留命令输出和 `/root/netkit/.netkit/audit.jsonl`。
