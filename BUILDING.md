Build Instructions

Prerequisites
- Go: 1.24.x (required by go.mod)
- Git: required for fetching dependencies in CI/containers
- CGO: enabled for GUI builds (m17-message)
- macOS packaging: building the .app bundle requires running on macOS; code signing/notarization not included.

Dependencies (by environment)
- macOS (native GUI + CLI)
  - Xcode Command Line Tools: `xcode-select --install`
  - Go 1.24.x: via `brew install go` or official pkg
  - No additional Homebrew packages typically required (macOS provides OpenGL headers)
  - Optional: `brew install pkg-config` for general dev

- Linux (native GUI + CLI)
  - Go 1.24.x (e.g., `sudo snap install go --channel=1.24/stable` or tarball)
  - GUI (GLFW/OpenGL) headers and X11 dev libs (Debian/Ubuntu):
    - `sudo apt-get update && sudo apt-get install -y \
      xorg-dev libgl1-mesa-dev libglu1-mesa-dev \
      libxrandr-dev libxcursor-dev libxinerama-dev libxi-dev`
  - CLI only: standard build-essential + Go

- Windows (GUI + CLI)
  - Recommended: use CI or containerized cross-build from Linux (mingw-w64)
  - Native build (advanced):
    - Install MSYS2, then within MSYS2 MinGW64 shell:
      - `pacman -S --needed mingw-w64-x86_64-go mingw-w64-x86_64-pkg-config \
         mingw-w64-x86_64-glew mingw-w64-x86_64-glfw`
    - Ensure CGO is enabled and MinGW toolchain is on PATH

- Raspberry Pi (CLI; GUI optional)
  - Go 1.24.x for arm64/armhf (tarball or apt backports)
  - CLI: no extra deps
  - GUI (optional; native Pi only): same X11/GL packages as Linux native

- Containerized builds (colima/Docker)
  - Docker or colima installed locally
  - For buildx (amd64 builder): `docker buildx create --use` (or use GitHub Actions which sets this for you)
  - Dockerfile installs:
    - Linux GUI deps: `xorg-dev`, Mesa GL/GLU + X11 dev libs
    - Windows cross: `mingw-w64`
    - Linux amd64 cross-CC: `gcc-x86-64-linux-gnu`/`g++-x86-64-linux-gnu`

Layout
- Apps (binaries):
  - cmd/m17-gateway
  - cmd/m17-message (GUI)
  - cmd/m17-text-cli
- Libraries:
  - pkg/protocol, pkg/phy, pkg/modem, pkg/relay, pkg/hostfile, pkg/fec

Quick Start (native build)
1) cd m17
2) make tidy
3) make native

Outputs
- Binaries are placed under bin/<os>-<arch>/
  - Example on Apple Silicon: bin/darwin-arm64/{m17-gateway,m17-message,m17-text-cli,modem-emulator}
- macOS GUI app bundle: bin/darwin-<arch>/M17 Message.app

Cross-Compilation
- The Makefile includes common targets:
  - make cross
    - Builds for: darwin/{amd64,arm64}, linux/{amd64,armv7,armv6,arm64}, windows/amd64
    - Artifacts under bin/<os>-<arch>/ (for ARM: bin/linux-armv7, bin/linux-armv6)

Make Targets (summary)
- make tidy: go mod tidy for the repo
- make native: build all apps for host OS/arch, package macOS .app
- make test: run unit tests across all packages
- make cross: containerized cross-builds for all targets (GUI+CLI for desktop where supported)
- make clean: remove bin/
- make docker-image: build container image for non-GUI cross builds
- make docker-cross: containerized non-GUI builds for Linux/Windows and all Raspberry Pi targets
- make docker-image-amd64: build amd64 builder image (via buildx) for desktop GUI cross-builds
- make docker-cross-desktop: build Linux amd64 and Windows amd64 (GUI + CLI) in the amd64 builder
- make cross-host: (optional) run host-based cross builds (requires local cross-compilers for CGO targets)

Containerized Builds (default for make cross)
- make cross invokes both container pipelines so you don’t need local cross toolchains:
  1) Non‑GUI: `make docker-image && make docker-cross`
     - Builds Linux (amd64/arm64/armv7/armv6) and Windows (amd64) CLI apps + modem emulator
  2) Desktop GUI + CLI: `make docker-image-amd64 && make docker-cross-desktop`
     - Builds Linux amd64 and Windows amd64 GUI (m17-message) and CLI apps using an amd64 builder image
- Notes:
  - On Apple Silicon/macOS: colima is sufficient; Docker Desktop is not required
  - The Makefile ensures a buildx builder exists for the amd64 image
  - Raspberry Pi GUI builds are intentionally skipped; CLI tools are built for all Pi targets

GUI Builds on Linux/Windows
- The GUI app (m17-message) builds for:
  - macOS (native): packaged as .app automatically
  - Linux amd64 and Windows amd64 (containerized desktop job) — `make docker-image-amd64 && make docker-cross-desktop`
- Prerequisites:
  - CGO enabled with appropriate toolchains/SDKs and OpenGL dependencies as required by fyne/glfw.
  - If unavailable, the build system logs a warning and skips the GUI for that target; other apps still build.
- GUI is intentionally skipped for Raspberry Pi targets (linux/arm64, linux/armv7, linux/armv6).

Linux arm64 GUI
- Cross-building Linux arm64 GUI (CGO + OpenGL) in containers is brittle; use a native arm64 host or CI runner.
- In CI, a job (linux_arm64_gui) builds the GUI only when a native Linux ARM64 runner is available.
- Locally on a native arm64 Linux system:
  - sudo apt-get install -y xorg-dev libgl1-mesa-dev libglu1-mesa-dev libxrandr-dev libxcursor-dev libxinerama-dev libxi-dev
  - cd m17 && GOOS=linux GOARCH=arm64 CGO_ENABLED=1 go build -tags release -o bin/linux-arm64/m17-message ./cmd/m17-message

Raspberry Pi Targets
- Raspberry Pi 4/3 64-bit OS: linux/arm64 (already included)
- Raspberry Pi 4/3 32-bit OS: linux/armv7 (GOARM=7)
- Raspberry Pi Zero/1: linux/armv6 (GOARM=6)
- Build all via: make cross
- Or specifically: from m17 run
  - ./scripts/build_all.sh "m17-gateway m17-message m17-text-cli" "linux/armv7" bin
  - ./scripts/build_all.sh "m17-gateway m17-message m17-text-cli" "linux/armv6" bin
  - Note: m17-message (GUI) is skipped on Raspberry Pi targets.

macOS Packaging
- make native on macOS also creates M17 Message.app with a minimal Info.plist and Icon.png.
- The app is unsigned; for distribution you may need to sign and notarize.

Cleaning
- make clean

Host-based cross-builds (optional/advanced)
- If you prefer to cross-build without containers: `make cross-host`
  - Requires local cross compilers for CGO-enabled targets (e.g., `x86_64-linux-gnu-gcc` on macOS for linux/amd64)
  - On macOS, installing and wiring correct cross toolchains is non-trivial; containerized builds are strongly recommended
  - You can disable CGO for non-GUI builds via `CGO_ENABLED=0 make cross-host`, but some dependencies may still require CGO

Notes
- The GUI build uses fyne; if cross-compiling fails due to missing GUI toolchain/SDK, the build script will warn and skip that target only.
- You can extend TARGETS in Makefile to include more architectures (e.g., windows/arm64) if needed.
 - CI (release workflow) builds and packages:
   - macOS GUI + CLI (native runner), zipped .app and binaries
   - Linux amd64 + Windows amd64 GUI + CLI (amd64 builder image)
   - Linux/Windows non-GUI + Raspberry Pi targets (containerized)
   - Versioned tar.gz and zip archives with SHA256SUMS.txt

Troubleshooting
- CGO compiler not found (e.g., `x86_64-linux-gnu-gcc not found`): use `make cross` (containerized) or install cross compilers locally
- GUI build fails with missing GL/X11 headers: install packages listed in Dependencies (by environment) or use `make docker-cross-desktop`
- On arm64 hosts, emulated amd64 GUI builds can be flaky; use the amd64 builder (`make docker-cross-desktop`) or CI
