#!/usr/bin/env bash
# Copyright 2009 The Go Authors. All rights reserved.
# Use of this source code is governed by a BSD-style
# license that can be found in the LICENSE file.


# 编译流程./make.bash -v:
#   1. 设置各种环境变量，找到一个已有go
#   2. 用这个go正常编译./cmd/dist/，生成./cmd/dist/dist
#   3. ./cmd/dist/dist bootstrap -a -v
#   4. 由dist完成其他全部流程

# See golang.org/s/go15bootstrap for an overview of the build process.

# Environment variables that control make.bash:
#
# GOROOT_FINAL: The expected final Go root, baked into binaries.
# The default is the location of the Go tree during the build.
#
# GOHOSTARCH: The architecture for host tools (compilers and
# binaries).  Binaries of this type must be executable on the current
# system, so the only common reason to set this is to set
# GOHOSTARCH=386 on an amd64 machine.
#
# GOARCH: The target architecture for installed packages and tools.
#
# GOOS: The target operating system for installed packages and tools.
#
# GO_GCFLAGS: Additional go tool compile arguments to use when
# building the packages and commands.
#
# GO_LDFLAGS: Additional go tool link arguments to use when
# building the commands.
#
# CGO_ENABLED: Controls cgo usage during the build. Set it to 1
# to include all cgo related files, .c and .go file with "cgo"
# build directive, in the build. Set it to 0 to ignore them.
#
# GO_EXTLINK_ENABLED: Set to 1 to invoke the host linker when building
# packages that use cgo.  Set to 0 to do all linking internally.  This
# controls the default behavior of the linker's -linkmode option.  The
# default value depends on the system.
#
# GO_LDSO: Sets the default dynamic linker/loader (ld.so) to be used
# by the internal linker.
#
# CC: Command line to run to compile C code for GOHOSTARCH.
# Default is "gcc". Also supported: "clang".
#
# CC_FOR_TARGET: Command line to run to compile C code for GOARCH.
# This is used by cgo.  Default is CC.
#
# CXX_FOR_TARGET: Command line to run to compile C++ code for GOARCH.
# This is used by cgo. Default is CXX, or, if that is not set,
# "g++" or "clang++".
#
# FC: Command line to run to compile Fortran code for GOARCH.
# This is used by cgo. Default is "gfortran".
#
# PKG_CONFIG: Path to pkg-config tool. Default is "pkg-config".
#
# GO_DISTFLAGS: extra flags to provide to "dist bootstrap".
# (Or just pass them to the make.bash command line.)
#
# GOBUILDTIMELOGFILE: If set, make.bash and all.bash write
# timing information to this file. Useful for profiling where the
# time goes when these scripts run.
#
# GOROOT_BOOTSTRAP: A working Go tree >= Go 1.4 for bootstrap.
# If $GOROOT_BOOTSTRAP/bin/go is missing, $(go env GOROOT) is
# tried for all "go" in $PATH. $HOME/go1.4 by default.

set -e

# go env
# go help environment

# GOENV指向一个文件路径，go会从这个文件读取env配置
export GOENV=off

# 最终安装路径应该是GOROOT_FINAL/go, 如果有GOBIN会安装到这个路径
unset GOBIN # Issue 14340
unset GOFLAGS
unset GO111MODULE

if [ ! -f run.bash ]; then
	echo 'make.bash must be run from $GOROOT/src' 1>&2
	exit 1
fi

if [ "$GOBUILDTIMELOGFILE" != "" ]; then
	echo $(LC_TIME=C date) start make.bash >"$GOBUILDTIMELOGFILE"
fi

# Test for Windows.
case "$(uname)" in
*MINGW* | *WIN32* | *CYGWIN*)
	echo 'ERROR: Do not use make.bash to build on Windows.'
	echo 'Use make.bat instead.'
	echo
	exit 1
	;;
esac

# Test for bad ld.
if ld --version 2>&1 | grep 'gold.* 2\.20' >/dev/null; then
	echo 'ERROR: Your system has gold 2.20 installed.'
	echo 'This version is shipped by Ubuntu even though'
	echo 'it is known not to work on Ubuntu.'
	echo 'Binaries built with this linker are likely to fail in mysterious ways.'
	echo
	echo 'Run sudo apt-get remove binutils-gold.'
	echo
	exit 1
fi

# Test for bad SELinux.
# On Fedora 16 the selinux filesystem is mounted at /sys/fs/selinux,
# so loop through the possible selinux mount points.
# allow_execstack: Allow unconfined executables to make their stack executable
for se_mount in /selinux /sys/fs/selinux
do
	if [ -d $se_mount -a -f $se_mount/booleans/allow_execstack -a -x /usr/sbin/selinuxenabled ] && /usr/sbin/selinuxenabled; then
		if ! cat $se_mount/booleans/allow_execstack | grep -c '^1 1$' >> /dev/null ; then
			echo "WARNING: the default SELinux policy on, at least, Fedora 12 breaks "
			echo "Go. You can enable the features that Go needs via the following "
			echo "command (as root):"
			echo "  # setsebool -P allow_execstack 1"
			echo
			echo "Note that this affects your system globally! "
			echo
			echo "The build will continue in five seconds in case we "
			echo "misdiagnosed the issue..."

			sleep 5
		fi
	fi
done

# Test for debian/kFreeBSD.
# cmd/dist will detect kFreeBSD as freebsd/$GOARCH, but we need to
# disable cgo manually.
if [ "$(uname -s)" = "GNU/kFreeBSD" ]; then
	export CGO_ENABLED=0
fi

# [Requesting program interpreter: /lib64/ld-linux-x86-64.so.2] => GO_LDSO=/lib64/ld-linux-x86-64.so.2
# Test which linker/loader our system is using, if GO_LDSO is not set.
if [ -z "$GO_LDSO" ] && type readelf >/dev/null 2>&1; then
	if echo "int main() { return 0; }" | ${CC:-cc} -o ./test-musl-ldso -x c - >/dev/null 2>&1; then
		LDSO=$(readelf -l ./test-musl-ldso | grep 'interpreter:' | sed -e 's/^.*interpreter: \(.*\)[]]/\1/') >/dev/null 2>&1
		[ -z "$LDSO" ] || export GO_LDSO="$LDSO"
		rm -f ./test-musl-ldso
	fi
fi

# Clean old generated file that will cause problems in the build.
rm -f ./runtime/runtime_defs.go

# Finally!  Run the build.

verbose=false
vflag=""
if [ "$1" = "-v" ]; then
	verbose=true
	vflag=-v
	shift
fi

export GOROOT_BOOTSTRAP=${GOROOT_BOOTSTRAP:-$HOME/go1.4}
export GOROOT="$(cd .. && pwd)"

# 遍历PATH中的所有go，找到第一个默认GOROOT和当前需要编译的版本路径不一样的go，设置
# GOROOT_BOOTSTRAP为这个go的默认GOROOT, 结果为GOROOT_BOOTSTRAP=/usr/lib/go
IFS=$'\n'; for go_exe in $(type -ap go); do
	if [ ! -x "$GOROOT_BOOTSTRAP/bin/go" ]; then
		goroot=$(GOROOT='' GOOS='' GOARCH='' "$go_exe" env GOROOT)
		if [ "$goroot" != "$GOROOT" ]; then
			GOROOT_BOOTSTRAP=$goroot
		fi
	fi

done; unset IFS
if [ ! -x "$GOROOT_BOOTSTRAP/bin/go" ]; then
	echo "ERROR: Cannot find $GOROOT_BOOTSTRAP/bin/go." >&2
	echo "Set \$GOROOT_BOOTSTRAP to a working Go tree >= Go 1.4." >&2
	exit 1
fi

# Get the exact bootstrap toolchain version to help with debugging.
# We clear GOOS and GOARCH to avoid an ominous but harmless warning if
# the bootstrap doesn't support them.
GOROOT_BOOTSTRAP_VERSION=$(GOOS= GOARCH= $GOROOT_BOOTSTRAP/bin/go version | sed 's/go version //')
echo "Building Go cmd/dist using $GOROOT_BOOTSTRAP. ($GOROOT_BOOTSTRAP_VERSION)"
if $verbose; then
	echo cmd/dist
fi
if [ "$GOROOT_BOOTSTRAP" = "$GOROOT" ]; then
	echo "ERROR: \$GOROOT_BOOTSTRAP must not be set to \$GOROOT" >&2
	echo "Set \$GOROOT_BOOTSTRAP to a working Go tree >= Go 1.4." >&2
	exit 1
fi
rm -f cmd/dist/dist
GOROOT="$GOROOT_BOOTSTRAP" GOOS="" GOARCH="" GO111MODULE=off "$GOROOT_BOOTSTRAP/bin/go" build -o cmd/dist/dist ./cmd/dist

# -e doesn't propagate out of eval, so check success by hand.
#eval $(./cmd/dist/dist env -p || echo FAIL=true)
if [ "$FAIL" = true ]; then
	exit 1
fi

if $verbose; then
	echo
fi

if [ "$1" = "-d" ]; then
	env | grep GO
	shift
	exit 0
fi

if [ "$1" = "--dist-tool" ]; then
	# Stop after building dist tool.
	mkdir -p "$GOTOOLDIR"
	if [ "$2" != "" ]; then
		cp cmd/dist/dist "$2"
	fi
	mv cmd/dist/dist "$GOTOOLDIR"/dist
	exit 0
fi

# Run dist bootstrap to complete make.bash.
# Bootstrap installs a proper cmd/dist, built with the new toolchain.
# Throw ours, built with Go 1.4, away after bootstrap.

if $verbose; then
	echo "####### Run ./cmd/dist/dist env -p #######"
	./cmd/dist/dist env -p
	echo "##########################################"
fi

./cmd/dist/dist bootstrap -a $vflag $GO_DISTFLAGS "$@"
rm -f ./cmd/dist/dist

# DO NOT ADD ANY NEW CODE HERE.
# The bootstrap+rm above are the final step of make.bash.
# If something must be added, add it to cmd/dist's cmdbootstrap,
# to avoid needing three copies in three different shell languages
# (make.bash, make.bat, make.rc).
