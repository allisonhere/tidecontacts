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

.PHONY: plugin
plugin: build
	mkdir -p bin/plugins/tidecontacts
	go build -buildvcs=false -o bin/plugins/tidecontacts/tidecontacts-plugin ./cmd/tidecontacts-plugin
	cp plugins/tidemail/plugin.toml bin/plugins/tidecontacts/plugin.toml
	cp bin/tidecontacts bin/plugins/tidecontacts/tidecontacts

.PHONY: install-tidemail-plugin
install-tidemail-plugin: plugin
	python3 scripts/install_tidemail_plugin.py

.PHONY: tidemail-demo-build
# Each sibling is a separate Go module; run builds from its own directory.
tidemail-demo-build: plugin
	cd ../tidemail && go build -buildvcs=false -o ../tidecontact/bin/tidemail .
	cd ../tidemail && go build -buildvcs=false -o ../tidecontact/bin/tidemail-demo-seed ./cmd/tidecontacts-demo-seed

.PHONY: smoke-cli
smoke-cli: build
	python3 scripts/cli_smoke.py

.PHONY: extension-demo
extension-demo:
	mkdir -p bin/extensions/relationship-digest
	go build -buildvcs=false -o bin/extensions/relationship-digest/relationship-digest ./examples/extensions/relationship-digest
	cp examples/extensions/relationship-digest/extension.toml bin/extensions/relationship-digest/

.PHONY: smoke-extensions
smoke-extensions: build
	python3 scripts/extensions_smoke.py

.PHONY: smoke-mail-clients
smoke-mail-clients: build
	python3 scripts/mail_clients_smoke.py

.PHONY: smoke-directories
smoke-directories: plugin
	python3 scripts/directory_smoke.py
