ifeq ($(shell uname -s), Darwin)
    shasum=shasum -a256
else
    shasum=sha256sum
endif

name=kpatch
version=$(shell git describe --all --dirty --long | awk -F"-|/" '/^heads/ {print $$2 "." substr($$4, 2)}; /^tags/ { print $$2 }')
build_args=-ldflags "-X main.versionString=$(version)"

.PHONY: test dev-deps

all: test build checksums

build: build-linux build-darwin build-windows

build-linux: build/$(name)-$(version)-linux-amd64
build/$(name)-$(version)-linux-amd64: *.go glide.lock
	GOARCH=amd64 GOOS=linux go build -o $@ $(build_args)

build-darwin: build/$(name)-$(version)-darwin-amd64
build/$(name)-$(version)-darwin-amd64: *.go glide.lock
	GOARCH=amd64 GOOS=darwin go build -o $@ $(build_args)

build-windows: build/$(name)-$(version)-windows-amd64
build/$(name)-$(version)-windows-amd64: *.go glide.lock
	GOARCH=amd64 GOOS=windows go build -o $@ $(build_args)

checksums: build
	cd build/ && ${shasum} * > $(name)-$(version)-SHA256SUMS

dev-deps:
	go get github.com/Masterminds/glide
	go get -u github.com/golangci/golangci-lint/cmd/golangci-lint
	glide install --strip-vendor

test:
	golangci-lint run
	go test

clean:
	rm build/*
