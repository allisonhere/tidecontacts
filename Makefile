.PHONY: build install test check fmt
build:
	go build -buildvcs=false -o bin/tidecontacts ./cmd/tidecontacts
install:
	go install -buildvcs=false ./cmd/tidecontacts
test:
	go test ./...
fmt:
	gofmt -w cmd internal
check:
	test -z "$$(gofmt -l cmd internal)"
	go build -buildvcs=false ./cmd/tidecontacts
	go vet ./...
	go test -race -short ./...

.PHONY: smoke
smoke: build
	python3 scripts/terminal_smoke.py ./bin/tidecontacts
	python3 scripts/contacts_smoke.py ./bin/tidecontacts
	python3 scripts/crm_smoke.py ./bin/tidecontacts
