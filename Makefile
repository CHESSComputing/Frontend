#GOPATH:=$(PWD):${GOPATH}
#export GOPATH
flags=-ldflags="-s -w"
# flags=-ldflags="-s -w -extldflags -static"

# latest version tag without the leading "v" (version sort: 1.10 > 1.9)
TAG := $(shell git tag --sort=-v:refname 2>/dev/null | head -n 1 | sed -e 's,^v,,')
# value substituted for {{VERSION}} in main.go (unchanged when there is no tag)
VER := $(if $(TAG),$(TAG),{{VERSION}})

HOSTOS   := $(shell go env GOOS)
HOSTARCH := $(shell go env GOARCH)

GOLIB    ?= ../golib
# module path from go.mod, without a trailing /vN major-version suffix
MODULE   := $(shell go list -m 2>/dev/null | sed -E 's|/v[0-9]+$$||')
SRV      ?= $(notdir $(MODULE))
REPO     ?= https://$(MODULE)
REF      ?= main
DOCS_OUT ?= docs/ENDPOINTS.md

# every cross-platform binary produced by build_all (names are kept as before)
PLATFORM_TARGETS := \
	build_darwin_amd64 build_darwin_arm64 \
	build_amd64 build_amd64_static build_arm64 build_arm build_386 \
	build_power8 build_s390x build_riscv64 \
	build_freebsd_amd64 \
	build_windows_amd64 build_windows_arm64
OUTPUTS := srv srv_darwin_amd64 srv_darwin_arm64 \
	srv_amd64 srv_amd64_static srv_arm64 srv_arm srv_386 \
	srv_power8 srv_s390x srv_riscv64 srv_freebsd_amd64 \
	srv_amd64.exe srv_arm64.exe

.PHONY: all golib gorelease docs build build_all $(PLATFORM_TARGETS) \
	    install clean changes test test_code
# main.go is temporarily rewritten while building, so builds must not overlap
.NOTPARALLEL:

all: golib build

golib:
	./get_golib.sh

gorelease:
	goreleaser release --snapshot --clean

docs:
	@echo "generating $(DOCS_OUT) for $(SRV) ($(REPO), ref $(REF)) ..."
	mkdir -p $(dir $(DOCS_OUT))
	go run $(GOLIB)/tools/gendocs/main.go -root . -service $(SRV) -ref $(REF) -repo $(REPO) -out $(DOCS_OUT)

# $(call xbuild,GOOS,GOARCH,OUTPUT[,ENV_VARS][,EXTRA_GO_FLAGS])
# - injects the version into main.go from a backup copy
# - always restores main.go, even if the build fails
# - exits with the exit code of "go build"
define xbuild
	@echo "==> building $(3) for $(1)/$(2) architecture"
	@rm -f $(3)
	@cp main.go main.go.orig; \
	sed -e "s,{{VERSION}},$(VER),g" main.go.orig > main.go; \
	$(4) GOOS=$(1) GOARCH=$(2) go build $(5) $(flags) -o $(3); rc=$$?; \
	mv main.go.orig main.go; \
	exit $$rc
endef

# native build for the current machine
build:
	$(call xbuild,$(HOSTOS),$(HOSTARCH),srv)

build_all: golib $(PLATFORM_TARGETS) changes docs

# macOS
build_darwin_amd64:
	$(call xbuild,darwin,amd64,srv_darwin_amd64)
build_darwin_arm64:
	$(call xbuild,darwin,arm64,srv_darwin_arm64)

# Linux
build_amd64:
	$(call xbuild,linux,amd64,srv_amd64)
build_amd64_static:
	$(call xbuild,linux,amd64,srv_amd64_static,CGO_ENABLED=0,-tags static)
build_arm64:
	$(call xbuild,linux,arm64,srv_arm64)
build_arm:
	$(call xbuild,linux,arm,srv_arm,GOARM=7)
build_386:
	$(call xbuild,linux,386,srv_386)
build_power8:
	$(call xbuild,linux,ppc64le,srv_power8)
build_s390x:
	$(call xbuild,linux,s390x,srv_s390x)
build_riscv64:
	$(call xbuild,linux,riscv64,srv_riscv64)

# FreeBSD
build_freebsd_amd64:
	$(call xbuild,freebsd,amd64,srv_freebsd_amd64)

# Windows
build_windows_amd64:
	$(call xbuild,windows,amd64,srv_amd64.exe)
build_windows_arm64:
	$(call xbuild,windows,arm64,srv_arm64.exe)

install:
	go install

clean:
	go clean; rm -rf pkg; rm -f $(OUTPUTS) main.go.orig

changes:
	./changes.sh
	./last_changes.sh

test : test_code

test_code:
	touch ~/.foxden.yaml
	go test -test.v .

# here is an example for execution of individual test
# go test -v -run TestFilesDB
