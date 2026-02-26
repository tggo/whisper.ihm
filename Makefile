WHISPER_DIR   := whisper.cpp
BUILD_DIR     := $(WHISPER_DIR)/build
TEN_VAD_DIR   := ten-vad
MODEL_DIR     := models
MODEL         := $(MODEL_DIR)/ggml-large-v3.bin
BINARY        := whisper-ihm

CGO_ENV := C_INCLUDE_PATH=$(CURDIR)/$(WHISPER_DIR)/include:$(CURDIR)/$(WHISPER_DIR)/ggml/include \
           LIBRARY_PATH=$(CURDIR)/$(BUILD_DIR)/src:$(CURDIR)/$(BUILD_DIR)/ggml/src:$(CURDIR)/$(BUILD_DIR)/ggml/src/ggml-metal:$(CURDIR)/$(BUILD_DIR)/ggml/src/ggml-blas \
           CGO_LDFLAGS="-lwhisper -lggml -lggml-base -lggml-cpu -lggml-blas -lggml-metal -lm -lstdc++ -framework Accelerate -framework Metal -framework Foundation -framework CoreGraphics"

.PHONY: all setup build test clean distclean

all: setup build

setup: $(BUILD_DIR)/src/libwhisper.a $(MODEL) $(TEN_VAD_DIR)

$(WHISPER_DIR):
	git clone --depth 1 https://github.com/ggml-org/whisper.cpp.git $(WHISPER_DIR)

$(BUILD_DIR)/src/libwhisper.a: | $(WHISPER_DIR)
	cmake -S $(WHISPER_DIR) -B $(BUILD_DIR) \
		-DCMAKE_BUILD_TYPE=Release \
		-DGGML_METAL=ON \
		-DGGML_METAL_EMBED_LIBRARY=ON \
		-DBUILD_SHARED_LIBS=OFF
	cmake --build $(BUILD_DIR) --config Release -j$(shell sysctl -n hw.ncpu)

$(TEN_VAD_DIR):
	git clone --depth 1 https://github.com/TEN-framework/ten-vad.git $(TEN_VAD_DIR)

$(MODEL_DIR):
	mkdir -p $(MODEL_DIR)

$(MODEL): | $(MODEL_DIR)
	curl -L --progress-bar -o $(MODEL) \
		"https://huggingface.co/ggerganov/whisper.cpp/resolve/main/ggml-large-v3.bin"

build: $(BUILD_DIR)/src/libwhisper.a
	$(CGO_ENV) go mod tidy
	$(CGO_ENV) go build -o $(BINARY) .

test: build
	./$(BINARY) testdata/short.mp3

clean:
	rm -rf $(BUILD_DIR) $(BINARY)

distclean: clean
	rm -rf $(WHISPER_DIR) $(TEN_VAD_DIR) $(MODEL_DIR)
