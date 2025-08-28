SHELL := /bin/bash

# Applications to build
APPS := m17-gateway m17-message m17-text-cli modem-emulator

# Map app -> command path
APP_m17-gateway_DIR := cmd/m17-gateway
APP_m17-message_DIR := cmd/m17-message
APP_m17-text-cli_DIR := cmd/m17-text-cli
APP_modem-emulator_DIR := cmd/modem-emulator

# Default cross-platform targets (os/arch)
# Default cross-platform targets (add Raspberry Pi variants)
TARGETS := \
  darwin/amd64 darwin/arm64 \
  linux/amd64 linux/arm64 linux/armv7 linux/armv6 \
  windows/amd64

BIN_DIR := bin

.PHONY: all build native cross package-mac clean tidy test

all: tidy native test

# Tidy dependencies
tidy:
	go mod tidy

# Build for the host OS/ARCH into bin/$(GOOS)-$(GOARCH)
native:
	@./scripts/build_all.sh "$(APPS)" "$$(go env GOOS)/$$(go env GOARCH)" "$(BIN_DIR)"

# Cross-compile all apps for the defined TARGETS
# Cross-compile all apps using containerized builders so host toolchains are not required
cross:
	@echo "[INFO] Running containerized cross-builds (non-GUI + desktop GUI)"
	@$(MAKE) docker-image
	@$(MAKE) docker-cross
	@$(MAKE) docker-image-amd64
	@$(MAKE) docker-cross-desktop
	@echo "[INFO] Cross-build complete. Artifacts in $(BIN_DIR)/"

# Optional: previous host-based cross build (requires local cross-compilers for CGO targets)
.PHONY: cross-host
cross-host:
	@./scripts/build_all.sh "$(APPS)" "$(TARGETS)" "$(BIN_DIR)"

# Containerized cross-build (Linux + Windows + Raspberry Pi targets)
.PHONY: docker-image docker-cross docker-push docker-push-amd64

# Registry/Images (override as needed)
IMAGE_NS ?= ghcr.io/jki757
IMAGE_TAG ?= go1.24-bookworm-v1
IMAGE_BASE ?= $(IMAGE_NS)/m17-go-builder
IMAGE_DESKTOP ?= $(IMAGE_NS)/m17-go-desktop-builder
docker-image:
	# Build base builder image (local + GHCR tags)
	docker build \
		-t m17-builder:latest \
		-t $(IMAGE_BASE):$(IMAGE_TAG) \
		-t $(IMAGE_BASE):latest \
		.

docker-cross: docker-image
	docker run --rm -e CI_CONTAINER=1 \
		-v $$(pwd):/work \
		-v $$HOME/.cache/m17-gomod:/go/pkg/mod \
		-v $$HOME/.cache/m17-gobuild:/root/.cache/go-build \
		-w /work $(IMAGE_BASE):$(IMAGE_TAG) \
		/bin/sh -lc "export PATH=/usr/local/go/bin:\$$PATH; ./scripts/build_all.sh '$(APPS)' 'linux/amd64 windows/amd64 linux/armv7 linux/armv6 linux/arm64' '$(BIN_DIR)'"

# Build amd64 builder image via buildx (so we can run desktop GUI cross-builds reliably)
.PHONY: docker-image-amd64 docker-cross-desktop
docker-image-amd64:
	@docker buildx inspect >/dev/null 2>&1 || docker buildx create --use
	docker buildx build --platform linux/amd64 \
		-t m17-builder-amd64:latest \
		-t $(IMAGE_DESKTOP):$(IMAGE_TAG) \
		-t $(IMAGE_DESKTOP):latest \
		--load .

# Containerized desktop builds (Linux amd64 and Windows amd64, with GUI)
docker-cross-desktop: docker-image-amd64
	docker run --rm --platform linux/amd64 -e CI_CONTAINER=1 -e ALLOW_GUI_CROSS=1 \
		-v $$(pwd):/work \
		-v $$HOME/.cache/m17-gomod:/go/pkg/mod \
		-v $$HOME/.cache/m17-gobuild:/root/.cache/go-build \
		-w /work $(IMAGE_DESKTOP):$(IMAGE_TAG) \
		/bin/sh -lc "export PATH=/usr/local/go/bin:\$$PATH; ./scripts/build_all.sh '$(APPS)' 'linux/amd64 windows/amd64' '$(BIN_DIR)'"

# Push images to GHCR (requires: docker login ghcr.io)
docker-push: docker-image
	docker push $(IMAGE_BASE):$(IMAGE_TAG)
	docker push $(IMAGE_BASE):latest

docker-push-amd64: docker-image-amd64
	docker push $(IMAGE_DESKTOP):$(IMAGE_TAG)
	docker push $(IMAGE_DESKTOP):latest

# Package macOS .app for m17-message for current arch
package-mac:
	@./scripts/package_mac_app.sh \
		"$(BIN_DIR)/$$(go env GOOS)-$$(go env GOARCH)/m17-message" \
		"$(BIN_DIR)/$$(go env GOOS)-$$(go env GOARCH)" \
		"M17 Message" \
		"com.jancona.m17.message" \
		"$(APP_m17-message_DIR)/Icon.png"

clean:
	rm -rf "$(BIN_DIR)"

# Run unit tests for all packages
test:
	go test ./...
