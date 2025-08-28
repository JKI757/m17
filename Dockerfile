FROM golang:1.24-bookworm AS builder

# Base OS packages for Linux GUI (GLFW/OpenGL) and Windows cross-compile
RUN apt-get update && DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends \
    build-essential ca-certificates git pkg-config \
    xorg-dev libgl1-mesa-dev libglu1-mesa-dev \
    libxrandr-dev libxcursor-dev libxinerama-dev libxi-dev \
    gcc-12 g++-12 \
    gcc-x86-64-linux-gnu g++-x86-64-linux-gnu \
    mingw-w64 \
    && rm -rf /var/lib/apt/lists/*

# Prefer newer GCC toolchain where available
RUN update-alternatives --install /usr/bin/gcc gcc /usr/bin/gcc-12 120 \
 && update-alternatives --install /usr/bin/g++ g++ /usr/bin/g++-12 120

WORKDIR /work

# Ensure Go is on PATH
ENV PATH=/usr/local/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin \
    CGO_ENABLED=1 \
    GOTOOLCHAIN=local

CMD ["bash"]
