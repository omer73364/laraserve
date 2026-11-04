VERSION ?= 0.1.0
LDFLAGS := -s -w -X laraserve/internal/runtime.Version=$(VERSION)

.PHONY: build test check check-ui dist release clean

# No PHP, Composer, MySQL or phpMyAdmin files are embedded or downloaded here.
build:
	go build -buildvcs=false -ldflags "$(LDFLAGS)" -o bin/laraserve .

test:
	go test ./...

check:
	go test ./...
	go vet ./...

check-ui:
	node --check internal/dashboard/assets/app.js
	node --check internal/dashboard/assets/controls.js
	node --check internal/dashboard/assets/notifications.js
	node --check internal/dashboard/assets/quick-open.js
	node --test scripts/tests/dashboard-state.test.cjs

dist:
	VERSION=$(VERSION) ./scripts/build-dist.sh

release: dist
	go run ./scripts/release --dir dist --version "$(VERSION)" --archives

clean:
	rm -rf bin dist
