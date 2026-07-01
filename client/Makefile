DIST_DIR := $(CURDIR)/dist
DESTDIR ?= $(CURDIR)/../output/client

.PHONY: all build install clean

all: build

build:
	pnpm install --frozen-lockfile
	pnpm run build

install:
	@test -d "$(DIST_DIR)" || (echo "client is not built; run make all first" && exit 1)
	mkdir -p $(DESTDIR)
	cp -R $(DIST_DIR)/. $(DESTDIR)/

clean:
	rm -rf $(DIST_DIR)
