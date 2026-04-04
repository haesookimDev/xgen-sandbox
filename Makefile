.PHONY: all build build-agent build-sidecar build-images dev-cluster dev-deploy dev-teardown test lint

# --- Build ---

all: build

build: build-agent build-sidecar

build-agent:
	cd agent && go build -o ../bin/agent ./cmd/agent

build-sidecar:
	cd sidecar && go build -o ../bin/sidecar ./cmd/sidecar

build-images:
	docker build -t ghcr.io/xgen-sandbox/agent:latest ./agent
	docker build -t ghcr.io/xgen-sandbox/sidecar:latest ./sidecar
	docker build -t ghcr.io/xgen-sandbox/runtime-base:latest ./runtime/base

build-sdk:
	cd sdks/typescript && npm install && npm run build

# --- Local Development ---

dev-cluster:
	kind create cluster --config deploy/dev/kind-config.yaml
	kind load docker-image ghcr.io/xgen-sandbox/agent:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/sidecar:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-base:latest --name xgen-sandbox

dev-deploy:
	helm upgrade --install xgen-sandbox deploy/helm/xgen-sandbox \
		--namespace xgen-system --create-namespace \
		--set agent.image.pullPolicy=Never \
		--set sidecar.image.pullPolicy=Never

dev-teardown:
	kind delete cluster --name xgen-sandbox

dev-reload: build-images
	kind load docker-image ghcr.io/xgen-sandbox/agent:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/sidecar:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-base:latest --name xgen-sandbox
	kubectl rollout restart deployment/xgen-agent -n xgen-system

# --- Test ---

test:
	cd agent && go test ./...
	cd sidecar && go test ./...

test-sdk:
	cd sdks/typescript && npm test

# --- Lint ---

lint:
	cd agent && go vet ./...
	cd sidecar && go vet ./...

# --- Go module management ---

tidy:
	cd agent && go mod tidy
	cd sidecar && go mod tidy
