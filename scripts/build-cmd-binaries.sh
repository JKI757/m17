#!/usr/bin/env sh
set -eu

ROOT_DIR="$(cd "$(dirname "$0")/.." && pwd)"

if ! command -v go >/dev/null 2>&1; then
    echo "Go toolchain not found in PATH" >&2
    exit 1
fi

BIN_ROOT="${ROOT_DIR}/bin"
HOST_OS="$(go env GOOS)"
HOST_ARCH="$(go env GOARCH)"
HOST_BIN_DIR="${BIN_ROOT}/${HOST_OS}-${HOST_ARCH}"
PI_BIN_DIR="${BIN_ROOT}/linux-armv7"

mkdir -p "${HOST_BIN_DIR}" "${PI_BIN_DIR}"

MAIN_PACKAGES=$(go list -f '{{if eq .Name "main"}}{{.ImportPath}}{{end}}' ./cmd/... | grep . || true)

if [ -z "${MAIN_PACKAGES}" ]; then
    echo "No main packages found under cmd/."
    exit 0
fi

SKIP_PI_PACKAGES="github.com/jancona/m17/cmd/m17-message"

for pkg in ${MAIN_PACKAGES}; do
    app_name=$(basename "${pkg}")
    echo "Building ${app_name} for ${HOST_OS}/${HOST_ARCH}..."
    go build -o "${HOST_BIN_DIR}/${app_name}" "${pkg}"

    case " ${SKIP_PI_PACKAGES} " in
        *" ${pkg} "*)
            echo "Skipping Raspberry Pi build for ${app_name} (unsupported dependencies)."
            ;;
        *)
            echo "Building ${app_name} for Raspberry Pi Zero 2 W (linux/armv7)..."
            GOOS=linux GOARCH=arm GOARM=7 go build -o "${PI_BIN_DIR}/${app_name}" "${pkg}"
            ;;
    esac

    if [ -f "${PI_BIN_DIR}/${app_name}" ]; then
        echo "Built ${app_name} -> ${HOST_BIN_DIR}/${app_name} and ${PI_BIN_DIR}/${app_name}"
    else
        echo "Built ${app_name} -> ${HOST_BIN_DIR}/${app_name}"
    fi
    echo
done

echo "All binaries are in ${HOST_BIN_DIR} and ${PI_BIN_DIR}."
