#!/bin/sh
# Netkit Linux 安装脚本：下载已发布的二进制，校验后安装到指定目录。
set -eu

repository="rustyllh/netkit"
install_dir="${NETKIT_INSTALL_DIR:-/usr/local/bin}"
version="${NETKIT_VERSION:-}"

fail() {
	printf '%s\n' "netkit install: $*" >&2
	exit 1
}

if [ "$(uname -s)" != "Linux" ]; then
	fail "仅支持 Linux"
fi

case "$(uname -m)" in
	x86_64 | amd64) architecture="amd64" ;;
	aarch64 | arm64) architecture="arm64" ;;
	*) fail "不支持的 CPU 架构: $(uname -m)" ;;
esac

if ! command -v curl >/dev/null 2>&1; then
	fail "需要 curl"
fi
if ! command -v sha256sum >/dev/null 2>&1; then
	fail "需要 sha256sum"
fi
if ! command -v tar >/dev/null 2>&1; then
	fail "需要 tar"
fi

if [ -z "$version" ]; then
	latest_url=$(curl -fsSL -o /dev/null -w '%{url_effective}' "https://github.com/${repository}/releases/latest") || fail "无法查询最新版本"
	version=${latest_url##*/}
fi
case "$version" in
	v*) ;;
	*) fail "版本必须以 v 开头，例如 v0.1.3" ;;
esac

package="netkit_linux_${architecture}.tar.gz"
release_url="https://github.com/${repository}/releases/download/${version}"
temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM

curl -fsSL "${release_url}/${package}" -o "${temporary_dir}/${package}" || fail "下载 ${package} 失败"
curl -fsSL "${release_url}/SHA256SUMS" -o "${temporary_dir}/SHA256SUMS" || fail "下载校验和失败"

(
	cd "$temporary_dir"
	grep -F "  ${package}" SHA256SUMS | sha256sum -c -
) || fail "SHA256 校验失败"
tar -xzf "${temporary_dir}/${package}" -C "$temporary_dir" || fail "解压 ${package} 失败"
[ -f "${temporary_dir}/netkit" ] || fail "压缩包中缺少 netkit"

if [ "$(id -u)" -eq 0 ]; then
	install -d -m 0755 "$install_dir"
	install -m 0755 "${temporary_dir}/netkit" "${install_dir}/netkit"
elif command -v sudo >/dev/null 2>&1; then
	sudo install -d -m 0755 "$install_dir"
	sudo install -m 0755 "${temporary_dir}/netkit" "${install_dir}/netkit"
else
	fail "安装到 ${install_dir} 需要 root 权限或 sudo"
fi

"${install_dir}/netkit" --version
