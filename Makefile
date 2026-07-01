CLIENT_DIR := $(CURDIR)/client
SERVER_DIR := $(CURDIR)/server
OUTPUT_DIR ?= $(CURDIR)/output
ARCH ?= amd64

PLATFORMS := windows mac linux

.PHONY: all client server install clean

all: client server

client:
	$(MAKE) -C $(CLIENT_DIR) all

server:
	$(MAKE) -C $(SERVER_DIR) all ARCH=$(ARCH)

install: all
	@for platform in $(PLATFORMS); do \
		$(MAKE) -C $(SERVER_DIR) install PLATFORM=$$platform ARCH=$(ARCH) DESTDIR=$(OUTPUT_DIR)/$$platform; \
		$(MAKE) -C $(CLIENT_DIR) install DESTDIR=$(OUTPUT_DIR)/$$platform/client; \
	done

clean:
	$(MAKE) -C $(CLIENT_DIR) clean
	$(MAKE) -C $(SERVER_DIR) clean
	rm -rf $(OUTPUT_DIR)
