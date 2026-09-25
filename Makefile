DB ?= postgres://vigil:vigil@localhost:5432/vigil?sslmode=disable

.PHONY: build test test-db run up down install uninstall open logs

build:
	go build -o bin/vigil ./cmd/vigil

test:
	go vet ./...
	go test -race ./...

test-db:
	VIGIL_TEST_DATABASE_URL="$(DB)" go test -race -count=1 ./...

run: build
	./bin/vigil -config vigil.yaml

up:
	docker compose -f deploy/docker-compose.yml up --build -d

down:
	docker compose -f deploy/docker-compose.yml down

PLIST := $(HOME)/Library/LaunchAgents/com.jkong7.vigil.plist
CONFIG := $(HOME)/.config/vigil/vigil.yaml

install: build
	mkdir -p $(HOME)/.local/bin $(HOME)/.config/vigil $(HOME)/Library/LaunchAgents
	install -m 755 bin/vigil $(HOME)/.local/bin/vigil
	test -f $(CONFIG) || cp vigil.local.example.yaml $(CONFIG)
	sed 's|__HOME__|$(HOME)|g' deploy/launchd/com.jkong7.vigil.plist > $(PLIST)
	launchctl bootout gui/$$(id -u) $(PLIST) 2>/dev/null || true
	launchctl bootstrap gui/$$(id -u) $(PLIST)
	@echo "vigil is running at http://127.0.0.1:7777 (config: $(CONFIG))"

uninstall:
	launchctl bootout gui/$$(id -u) $(PLIST) 2>/dev/null || true
	rm -f $(PLIST) $(HOME)/.local/bin/vigil

open:
	open http://127.0.0.1:7777

logs:
	tail -f $(HOME)/Library/Logs/vigil.log
