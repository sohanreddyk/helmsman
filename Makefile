.PHONY: run build test tidy fmt vet clean kill

run:
	go run ./cmd/gateway

build:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./...

tidy:
	go mod tidy

fmt:
	go fmt ./...

vet:
	go vet ./...

clean:
	rm -rf bin

kill:
	-lsof -ti :8080 | xargs kill -9 2>/dev/null || true

restart: kill run
