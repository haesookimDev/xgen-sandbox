-include .env
export

.PHONY: all build build-agent build-sidecar build-dashboard build-images dev-cluster dev-deploy dev-dashboard dev-teardown test lint security-scan help

# --- Help ---

help: ## Show available targets
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "  \033[36m%-20s\033[0m %s\n", $$1, $$2}'

# --- Build ---

all: build ## Build agent and sidecar binaries

build: build-agent build-sidecar ## Build agent and sidecar binaries

build-agent: ## Build agent binary
	cd agent && go build -o ../bin/agent ./cmd/agent

build-sidecar: ## Build sidecar binary
	cd sidecar && go build -o ../bin/sidecar ./cmd/sidecar

build-images: ## Build all Docker images
	docker build -t ghcr.io/xgen-sandbox/agent:latest ./agent
	docker build -t ghcr.io/xgen-sandbox/sidecar:latest ./sidecar
	docker build -t ghcr.io/xgen-sandbox/runtime-base:latest ./runtime/base
	docker build -t ghcr.io/xgen-sandbox/runtime-nodejs:latest ./runtime/nodejs
	docker build -t ghcr.io/xgen-sandbox/runtime-python:latest ./runtime/python
	docker build -t ghcr.io/xgen-sandbox/runtime-gui:latest ./runtime/gui
	docker build -t ghcr.io/xgen-sandbox/dashboard:latest ./dashboard

build-dashboard: ## Build Next.js dashboard
	cd dashboard && npm install && npm run build

build-sdk: ## Build TypeScript SDK
	cd sdks/typescript && npm install && npm run build

# --- Local Development ---

dev-cluster: ## Create Kind cluster and load images
	kind create cluster --config deploy/dev/kind-config.yaml
	kind load docker-image ghcr.io/xgen-sandbox/agent:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/sidecar:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-base:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-nodejs:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-python:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-gui:latest --name xgen-sandbox

dev-deploy: ## Deploy to Kind cluster via Helm
	helm upgrade --install xgen-sandbox deploy/helm/xgen-sandbox \
		--namespace xgen-system --create-namespace \
		--set agent.image.pullPolicy=Never \
		--set sidecar.image.pullPolicy=Never \
		--set sandbox.imagePullPolicy=Never \
		--set agent.service.type=NodePort \
		--set agent.secrets.apiKey=$(API_KEY) \
		--set agent.secrets.jwtSecret=$(JWT_SECRET)

dev-dashboard: ## Run dashboard dev server
	cd dashboard && npm run dev

dev-teardown: ## Delete Kind cluster
	kind delete cluster --name xgen-sandbox

dev-reload: build-images ## Rebuild images and restart agent in Kind
	kind load docker-image ghcr.io/xgen-sandbox/agent:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/sidecar:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-base:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-nodejs:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-python:latest --name xgen-sandbox
	kind load docker-image ghcr.io/xgen-sandbox/runtime-gui:latest --name xgen-sandbox
	kubectl rollout restart deployment/xgen-agent -n xgen-system

# --- Test ---

test: ## Run Go tests for agent and sidecar
	cd agent && go test ./...
	cd sidecar && go test ./...

test-sdk: ## Run TypeScript SDK tests
	cd sdks/typescript && npm test

# --- Lint ---

lint: ## Run Go vet on agent and sidecar
	cd agent && go vet ./...
	cd sidecar && go vet ./...

# --- Go module management ---

tidy: ## Run go mod tidy for agent and sidecar
	cd agent && go mod tidy
	cd sidecar && go mod tidy

# --- Security ---

security-scan: build-images ## Run Trivy security scan on Docker images (requires: brew install trivy)
	trivy image --severity CRITICAL,HIGH --exit-code 1 ghcr.io/xgen-sandbox/agent:latest
	trivy image --severity CRITICAL,HIGH --exit-code 1 ghcr.io/xgen-sandbox/sidecar:latest
	trivy image --severity CRITICAL,HIGH --exit-code 1 ghcr.io/xgen-sandbox/dashboard:latest

# --- Hot Reload Development ---

dev-agent: ## Run agent with hot reload (requires: go install github.com/air-verse/air@latest)
	cd agent && air -c ../.air.toml 2>/dev/null || go run ./cmd/agent
