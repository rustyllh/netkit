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

if [ "$(id -u)" -ne 0 ] && ! command -v sudo >/dev/null 2>&1; then
	fail "安装到 ${install_dir} 需要 root 权限或 sudo"
fi

install_file() {
	mode="$1"
	source_file="$2"
	destination_file="$3"
	if [ "$(id -u)" -eq 0 ]; then
		install -m "$mode" "$source_file" "$destination_file"
		return
	fi
	sudo install -m "$mode" "$source_file" "$destination_file"
}

install_directory() {
	if [ "$(id -u)" -eq 0 ]; then
		install -d -m 0755 "$1"
		return
	fi
	sudo install -d -m 0755 "$1"
}

install_completion() {
	source_file="$1"
	destination_dir="$2"
	destination_name="$3"
	if [ -f "$source_file" ] && [ -d "$destination_dir" ]; then
		install_file 0644 "$source_file" "${destination_dir}/${destination_name}"
	fi
}

configure_bash_completion() {
	loader="/usr/share/bash-completion/bash_completion"
	bashrc="/root/.bashrc"
	if [ ! -f "$loader" ]; then
		printf '%s\n' "netkit install: 未检测到 bash-completion，跳过 Bash 自动加载配置" >&2
		return
	fi
	if [ "$(id -u)" -eq 0 ]; then
		if grep -Fq "# >>> netkit bash completion >>>" "$bashrc" 2>/dev/null; then
			return
		fi
		cat >> "$bashrc" <<'EOF'

# >>> netkit bash completion >>>
if [ -f /usr/share/bash-completion/bash_completion ]; then
  . /usr/share/bash-completion/bash_completion
fi
# <<< netkit bash completion <<<
EOF
		return
	fi
	if sudo grep -Fq "# >>> netkit bash completion >>>" "$bashrc" 2>/dev/null; then
		return
	fi
	sudo tee -a "$bashrc" >/dev/null <<'EOF'

# >>> netkit bash completion >>>
if [ -f /usr/share/bash-completion/bash_completion ]; then
  . /usr/share/bash-completion/bash_completion
fi
# <<< netkit bash completion <<<
EOF
}

install_directory "$install_dir"
install_file 0755 "${temporary_dir}/netkit" "${install_dir}/netkit"
install_completion "${temporary_dir}/completions/netkit.bash" "/usr/share/bash-completion/completions" "netkit"
install_completion "${temporary_dir}/completions/_netkit" "/usr/share/zsh/vendor-completions" "_netkit"
install_completion "${temporary_dir}/completions/_netkit" "/usr/local/share/zsh/site-functions" "_netkit"
install_completion "${temporary_dir}/completions/netkit.fish" "/usr/share/fish/vendor_completions.d" "netkit.fish"
configure_bash_completion

"${install_dir}/netkit" --version
printf '%s\n' "重新登录，或执行: source /root/.bashrc，以启用 Bash 补全"
