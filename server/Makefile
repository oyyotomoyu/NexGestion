SERVER_NAME := nexgestion-server
BUILD_DIR := $(CURDIR)/build
ARCH ?= amd64
PLATFORM ?=
DESTDIR ?= $(CURDIR)/../output/$(PLATFORM)

.PHONY: all windows mac linux build-windows build-mac build-linux install clean test

all: windows mac linux

windows: build-windows

mac: build-mac

linux: build-linux

build-windows:
	mkdir -p $(BUILD_DIR)/windows
	CGO_ENABLED=0 GOOS=windows GOARCH=$(ARCH) go build -trimpath -o $(BUILD_DIR)/windows/$(SERVER_NAME).exe .

build-mac:
	mkdir -p $(BUILD_DIR)/mac
	CGO_ENABLED=0 GOOS=darwin GOARCH=$(ARCH) go build -trimpath -o $(BUILD_DIR)/mac/$(SERVER_NAME) .

build-linux:
	mkdir -p $(BUILD_DIR)/linux
	CGO_ENABLED=0 GOOS=linux GOARCH=$(ARCH) go build -trimpath -o $(BUILD_DIR)/linux/$(SERVER_NAME) .

install:
	@test -n "$(PLATFORM)" || (echo "PLATFORM is required (windows, mac, or linux)" && exit 1)
	@test -d "$(BUILD_DIR)/$(PLATFORM)" || (echo "unknown or unbuilt platform: $(PLATFORM)" && exit 1)
	mkdir -p $(DESTDIR)/server
	cp -R $(BUILD_DIR)/$(PLATFORM)/. $(DESTDIR)/server/

test:
	go test ./...

clean:
	rm -rf $(BUILD_DIR)
