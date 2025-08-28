#!/usr/bin/env bash
set -euo pipefail

APPS=${1:-"m17-gateway m17-message m17-text-cli"}
TARGETS=${2:-"darwin/amd64 darwin/arm64 linux/amd64 linux/arm64 linux/armv7 linux/armv6 windows/amd64"}
BIN_DIR=${3:-bin}

mkdir -p "$BIN_DIR"

cmd_dir() {
  local app="$1"
  case "$app" in
    m17-gateway) echo "cmd/m17-gateway" ;;
    m17-message) echo "cmd/m17-message" ;;
    m17-text-cli) echo "cmd/m17-text-cli" ;;
    modem-emulator) echo "cmd/modem-emulator" ;;
    *) echo "unknown app: $app" >&2; return 1 ;;
  esac
}

for target in $TARGETS; do
  IFS=/ read -r os arch var <<<"$target"
  outdir="$BIN_DIR/${os}-${arch}"
  mkdir -p "$outdir"
  echo "==> Building for ${os}/${arch} into ${outdir}"
  for app in $APPS; do
    dir=$(cmd_dir "$app")
    outfile="$outdir/$app"
    if [[ "$os" == "windows" ]]; then outfile+=".exe"; fi
    echo "go build -o $outfile ./$dir"

    # Handle Raspberry Pi (linux/armv6 or linux/armv7) and explicit GOARM via third field
    envs=("GOOS=$os" "GOARCH=$arch")
    if [[ "$arch" == armv7 ]]; then
      envs=("GOOS=$os" "GOARCH=arm" "GOARM=7")
      outdir="$BIN_DIR/${os}-armv7"; mkdir -p "$outdir"; outfile="$outdir/$app"; [[ "$os" == windows ]] && outfile+=".exe"
    elif [[ "$arch" == armv6 ]]; then
      envs=("GOOS=$os" "GOARCH=arm" "GOARM=6")
      outdir="$BIN_DIR/${os}-armv6"; mkdir -p "$outdir"; outfile="$outdir/$app"; [[ "$os" == windows ]] && outfile+=".exe"
    elif [[ -n "${var:-}" ]]; then
      # Pattern linux/arm/7
      if [[ "$arch" == arm ]]; then
        envs=("GOOS=$os" "GOARCH=arm" "GOARM=$var")
        outdir="$BIN_DIR/${os}-armv$var"; mkdir -p "$outdir"; outfile="$outdir/$app"; [[ "$os" == windows ]] && outfile+=".exe"
      fi
    fi
    # Toolchain env for cross CGO
    tool_env=()
    if [[ "$os" == "windows" ]]; then
      tool_env=("CC=x86_64-w64-mingw32-gcc" "CXX=x86_64-w64-mingw32-g++")
    elif [[ "$os" == "linux" && "$arch" == amd64 && "$(uname -m)" != x86_64 ]]; then
      tool_env=("CC=x86_64-linux-gnu-gcc" "CXX=x86_64-linux-gnu-g++")
    fi

    # Default CGO on, but disable for ARM Pi targets in containerized builds
    cgo_flag="CGO_ENABLED=1"
    if [[ "$os" == "linux" && ("$arch" == arm || "$arch" == armv7 || "$arch" == armv6) ]]; then
      cgo_flag="CGO_ENABLED=0"
    fi

    if [[ "$app" == "m17-message" ]]; then
      # GUI app: allow failure for cross-compiling if toolchain not available
      set +e
      # Skip GUI build on Raspberry Pi targets
      if [[ "$os" == "linux" && ("$arch" == arm || "$arch" == armv7 || "$arch" == armv6) ]]; then
        echo "Skipping GUI app for Raspberry Pi target $os/$arch"
        set -e
        continue
      fi
      # In containerized cross-builds, skip GUI on non-macOS targets by default
      if [[ -n "${CI_CONTAINER:-}" && "$os" != "darwin" && -z "${ALLOW_GUI_CROSS:-}" ]]; then
        echo "Skipping GUI app for containerized cross-build target $os/$arch"
        set -e
        continue
      fi
      # Try to enable CGO for desktop targets; requires appropriate SDKs/toolchains
      # Set cross-compiler for Windows and for linux/amd64 when building from arm64
      cc_env=()
      if [[ "$os" == "windows" ]]; then
        cc_env=("CC=x86_64-w64-mingw32-gcc" "CXX=x86_64-w64-mingw32-g++")
      elif [[ "$os" == "linux" && "$arch" == amd64 && "$(uname -m)" != x86_64 ]]; then
        cc_env=("CC=x86_64-linux-gnu-gcc" "CXX=x86_64-linux-gnu-g++")
      fi
      env "$cgo_flag" ${tool_env[@]+"${tool_env[@]}"} ${cc_env[@]+"${cc_env[@]}"} ${envs[@]+"${envs[@]}"} \
        go build -buildvcs=false -tags "release" -o "$outfile" "./$dir"
      rc=$?
      set -e
      if [[ $rc -ne 0 ]]; then
        echo "WARN: Skipping $app for ${os}/${arch} (missing toolchain/SDK)"
        continue
      fi
      if [[ "$os" == "darwin" ]]; then
        # Package macOS app bundle
        ./scripts/package_mac_app.sh "$outfile" "$outdir" "M17 Message" "com.jancona.m17.message" "$dir/Icon.png"
      fi
    else
      env "$cgo_flag" ${tool_env[@]+"${tool_env[@]}"} ${envs[@]+"${envs[@]}"} \
        go build -buildvcs=false -o "$outfile" "./$dir"
    fi
  done
done

echo "All builds completed. Artifacts in $BIN_DIR/"
