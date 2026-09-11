#!/bin/sh
# install.sh 的离线 Release fixture 测试。
set -eu

project_dir=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
temporary_dir=$(mktemp -d)
trap 'rm -rf "$temporary_dir"' EXIT HUP INT TERM

fail() {
	printf '%s\n' "install test: $*" >&2
	exit 1
}

make_release() {
	architecture="$1"
	release_dir="$temporary_dir/releases/v9.9.9"
	package_dir="$temporary_dir/package_${architecture}"
	mkdir -p "$release_dir" "$package_dir/completions"
	printf '#!/bin/sh\nprintf "netkit %s\\n"\n' "$architecture" > "$package_dir/netkit"
	chmod 0755 "$package_dir/netkit"
	printf '# bash completion\n' > "$package_dir/completions/netkit.bash"
	tar -C "$package_dir" -czf "$release_dir/netkit_linux_${architecture}.tar.gz" netkit completions
}

prepare_release() {
	make_release amd64
	make_release arm64
	(
		cd "$temporary_dir/releases/v9.9.9"
		sha256sum netkit_linux_amd64.tar.gz netkit_linux_arm64.tar.gz > SHA256SUMS
	)
}

make_uname() {
	architecture="$1"
	mkdir -p "$temporary_dir/bin"
	printf '%s\n' '#!/bin/sh' 'case "$1" in' '-s) printf "Linux\\n" ;;' "-m) printf '%s\\n' '$architecture' ;;" '*) exit 1 ;;' 'esac' > "$temporary_dir/bin/uname"
	chmod 0755 "$temporary_dir/bin/uname"
	printf '%s\n' '#!/bin/sh' 'if [ "$1" = "-u" ]; then' '  printf "0\\n"' 'fi' > "$temporary_dir/bin/id"
	chmod 0755 "$temporary_dir/bin/id"
}

run_install() {
	architecture="$1"
	case_dir="$2"
	make_uname "$architecture"
	PATH="$temporary_dir/bin:$PATH" \
	NETKIT_VERSION=v9.9.9 \
	NETKIT_RELEASE_BASE_URL="file://$temporary_dir/releases" \
	NETKIT_INSTALL_DIR="$case_dir/bin" \
	NETKIT_BASH_COMPLETION_DIR="$case_dir/bash-completions" \
	NETKIT_BASH_COMPLETION_LOADER="$case_dir/bash_completion" \
	NETKIT_BASHRC="$case_dir/bashrc" \
	sh "$project_dir/install.sh"
}

test_install_for_architecture() {
	architecture="$1"
	case_dir="$temporary_dir/$architecture"
	mkdir -p "$case_dir/bash-completions"
	printf '# loader\n' > "$case_dir/bash_completion"
	run_install "$architecture" "$case_dir"
	[ "$("$case_dir/bin/netkit" --version)" = "netkit $architecture" ] || fail "$architecture 安装了错误的压缩包"
	[ -f "$case_dir/bash-completions/netkit" ] || fail "$architecture 未安装 Bash 补全"
	grep -Fq '# >>> netkit bash completion >>>' "$case_dir/bashrc" || fail "$architecture 未配置 Bash 补全加载"
}

test_reinstall_is_idempotent() {
	case_dir="$temporary_dir/reinstall"
	mkdir -p "$case_dir/bash-completions"
	printf '# loader\n' > "$case_dir/bash_completion"
	run_install amd64 "$case_dir"
	run_install amd64 "$case_dir"
	[ "$(grep -Fc '# >>> netkit bash completion >>>' "$case_dir/bashrc")" -eq 1 ] || fail '重复安装写入了多份 Bash 配置'
}

test_checksum_failure() {
	case_dir="$temporary_dir/checksum-failure"
	mkdir -p "$case_dir/bash-completions"
	printf '%064d  netkit_linux_amd64.tar.gz\n' 0 > "$temporary_dir/releases/v9.9.9/SHA256SUMS"
	if run_install amd64 "$case_dir" >/dev/null 2>&1; then
		fail '校验和错误时安装不应成功'
	fi
	[ ! -e "$case_dir/bin/netkit" ] || fail '校验失败后仍安装了二进制'
	prepare_release
}

test_missing_checksum_entry() {
	case_dir="$temporary_dir/missing-checksum-entry"
	mkdir -p "$case_dir/bash-completions"
	printf 'invalid checksum\n' > "$temporary_dir/releases/v9.9.9/SHA256SUMS"
	if run_install amd64 "$case_dir" >/dev/null 2>&1; then
		fail '缺少校验和条目时安装不应成功'
	fi
	prepare_release
}

test_without_bash_completion() {
	case_dir="$temporary_dir/no-bash-completion"
	mkdir -p "$case_dir/bash-completions"
	run_install arm64 "$case_dir"
	[ ! -e "$case_dir/bashrc" ] || fail '缺少 bash-completion 时不应写入 Bash 配置'
}

prepare_release
test_install_for_architecture amd64
test_install_for_architecture arm64
test_reinstall_is_idempotent
test_checksum_failure
test_missing_checksum_entry
test_without_bash_completion
printf '%s\n' 'install fixture tests passed'
