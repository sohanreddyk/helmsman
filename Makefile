.PHONY: run build test tidy fmt vet clean kill restart docker-build k8s-up k8s-down k8s-status dashboard

run:
	go run ./cmd/gateway

build:
	go build -o bin/gateway ./cmd/gateway

test:
	go test ./... -v

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

docker-build:
	docker build -t helmsman:latest .

k8s-up:
	kubectl apply -f deploy/k8s/namespace.yaml
	kubectl apply -f deploy/k8s/redis.yaml
	kubectl apply -f deploy/k8s/configmap.yaml
	kubectl apply -f deploy/k8s/gateway.yaml

k8s-down:
	kubectl delete namespace helmsman

k8s-status:
	kubectl get all -n helmsman

dashboard:
	@echo "Opening dashboard at http://localhost:8081"
	@cd dashboard && python3 -m http.server 8081
