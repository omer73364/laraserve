VERSION ?= 0.1.0
LDFLAGS := -s -w -X laraserve/internal/runtime.Version=$(VERSION)

.PHONY: build test dist clean

# No PHP, Composer, MySQL or phpMyAdmin files are embedded or downloaded here.
build:
	go build -buildvcs=false -ldflags "$(LDFLAGS)" -o bin/laraserve .

test:
	go test ./...

dist:
	VERSION=$(VERSION) ./scripts/build-dist.sh

clean:
	rm -rf bin dist
