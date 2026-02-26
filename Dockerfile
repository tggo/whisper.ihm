FROM golang:1.23-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    cmake git ca-certificates libc++-dev libc++abi-dev \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src

# Clone dependencies
RUN git clone --depth 1 https://github.com/ggml-org/whisper.cpp.git whisper.cpp \
    && git clone --depth 1 https://github.com/TEN-framework/ten-vad.git ten-vad

# Build whisper.cpp (CPU only, no Metal/CUDA)
RUN cmake -S whisper.cpp -B whisper.cpp/build \
        -DCMAKE_BUILD_TYPE=Release \
        -DBUILD_SHARED_LIBS=OFF \
    && cmake --build whisper.cpp/build --config Release -j$(nproc)

# Copy source
COPY go.mod go.sum ./
RUN C_INCLUDE_PATH=/src/whisper.cpp/include:/src/whisper.cpp/ggml/include \
    LIBRARY_PATH=/src/whisper.cpp/build/src:/src/whisper.cpp/build/ggml/src \
    CGO_LDFLAGS="-lwhisper -lggml -lggml-base -lggml-cpu -lm -lstdc++" \
    go mod download

COPY *.go ./

RUN C_INCLUDE_PATH=/src/whisper.cpp/include:/src/whisper.cpp/ggml/include \
    LIBRARY_PATH=/src/whisper.cpp/build/src:/src/whisper.cpp/build/ggml/src \
    CGO_LDFLAGS="-lwhisper -lggml -lggml-base -lggml-cpu -lm -lstdc++" \
    CGO_ENABLED=1 \
    go build -trimpath -o /whisper-ihm .

# Runtime
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl libc++1 libc++abi1 \
    && rm -rf /var/lib/apt/lists/*

COPY --from=builder /src/ten-vad/lib/Linux/x64/libten_vad.so /usr/local/lib/
RUN ldconfig

COPY --from=builder /whisper-ihm /usr/local/bin/whisper-ihm

WORKDIR /data

ENTRYPOINT ["whisper-ihm"]
