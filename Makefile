VERSION ?= 0.1.0
LDFLAGS := -s -w -X laraserve/internal/runtime.Version=$(VERSION)

.PHONY: build test check dist release clean

# No PHP, Composer, MySQL or phpMyAdmin files are embedded or downloaded here.
build:
	go build -buildvcs=false -ldflags "$(LDFLAGS)" -o bin/laraserve .

test:
	go test ./...

check:
	go test ./...
	go vet ./...

dist:
	VERSION=$(VERSION) ./scripts/build-dist.sh

release: dist
	go run ./scripts/release --dir dist --version "$(VERSION)" --archives

clean:
	rm -rf bin dist
