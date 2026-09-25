DB ?= postgres://vigil:vigil@localhost:5432/vigil?sslmode=disable

.PHONY: build test test-db run up down

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
