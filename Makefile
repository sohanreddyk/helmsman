.PHONY: run build test tidy fmt vet clean

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
