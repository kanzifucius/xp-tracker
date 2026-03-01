.DEFAULT_GOAL := help

.PHONY: build test lint vet fmt run run-local run-local-namespace clean check ci \
        docker-build docker-build-multiarch docker-push docker-push-multiarch \
        deploy deploy-example deploy-dry-run \
        mod-tidy mod-verify \
        cover cover-html \
        samples-apply samples-delete \
        namespace-config-apply namespace-config-delete \
        kindplane-up kindplane-down kindplane-status dev dev-down \
        logs \
        docs-serve \
        help

BINARY    := xp-tracker
IMAGE     ?= ghcr.io/kanzifucius/xp-tracker
TAG       ?= latest
PLATFORMS ?= linux/amd64,linux/arm64

VERSION   ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo dev)
COMMIT    ?= $(shell git rev-parse --short HEAD 2>/dev/null || echo unknown)
BUILD_DATE?= $(shell date -u +%Y-%m-%dT%H:%M:%SZ)

LDFLAGS   := -s -w \
             -X main.version=$(VERSION) \
             -X main.commit=$(COMMIT) \
             -X main.date=$(BUILD_DATE)

# --- Build ---

build: ## Build the binary
	go build -ldflags="$(LDFLAGS)" -o bin/$(BINARY) ./cmd/exporter

run: build ## Build and run locally
	./bin/$(BINARY)

run-local: build ## Build and run locally with sample resource config
	CLAIM_GVRS=samples.xptracker.dev/v1alpha1/widgets,samples.xptracker.dev/v1alpha1/gadgets \
	XR_GVRS=samples.xptracker.dev/v1alpha1/xwidgets,samples.xptracker.dev/v1alpha1/xgadgets \
	CREATOR_ANNOTATION_KEY=xptracker.dev/created-by \
	TEAM_ANNOTATION_KEY=xptracker.dev/team \
	POLL_INTERVAL_SECONDS=10 \
	./bin/$(BINARY)

run-local-namespace: build ## Build and run locally using per-namespace ConfigMaps (no central GVRs)
	CREATOR_ANNOTATION_KEY=xptracker.dev/created-by \
	TEAM_ANNOTATION_KEY=xptracker.dev/team \
	POLL_INTERVAL_SECONDS=10 \
	./bin/$(BINARY)

clean: ## Remove build artifacts
	rm -rf bin/ coverage.out coverage.html

# --- Quality ---

fmt: ## Format code with gofmt
	gofmt -w -s .

vet: ## Run go vet
	go vet ./...

lint: ## Run golangci-lint
	golangci-lint run ./...

test: ## Run tests with race detector
	go test -race -count=1 ./...

cover: ## Run tests with coverage report
	go test -race -count=1 -coverprofile=coverage.out ./...
	go tool cover -func=coverage.out

cover-html: cover ## Open HTML coverage report in browser
	go tool cover -html=coverage.out -o coverage.html
	open coverage.html || xdg-open coverage.html 2>/dev/null

check: fmt vet lint test ## Run all checks (fmt, vet, lint, test)

ci: vet lint test ## CI-equivalent checks (vet, lint, test)

# --- Modules ---

mod-tidy: ## Tidy go.mod and go.sum
	go mod tidy

mod-verify: ## Verify module dependencies
	go mod verify

# --- Docker ---

docker-build: ## Build container image (single arch)
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE):$(TAG) .

docker-build-multiarch: ## Build multi-arch image (amd64 + arm64), no push
	docker buildx build --platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE):$(TAG) .

docker-push: ## Build and push container image (single arch)
	docker build \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE):$(TAG) .
	docker push $(IMAGE):$(TAG)

docker-push-multiarch: ## Build and push multi-arch image
	docker buildx build --platform $(PLATFORMS) \
		--build-arg VERSION=$(VERSION) \
		--build-arg COMMIT=$(COMMIT) \
		--build-arg BUILD_DATE=$(BUILD_DATE) \
		-t $(IMAGE):$(TAG) --push .

# --- Kustomize Deploy ---

deploy: ## Apply base kustomize manifests
	kubectl apply -k deploy/base

deploy-example: ## Apply example overlay manifests
	kubectl apply -k deploy/overlays/example

deploy-dry-run: ## Render base manifests to stdout (no apply)
	kubectl kustomize deploy/base

# --- Samples ---

samples-apply: ## Apply sample XRDs, Compositions, and Claims to the cluster
	kubectl apply -f hack/samples/namespaces.yaml
	kubectl apply -f hack/samples/functions.yaml
	kubectl apply -f hack/samples/xrds.yaml
	@echo "Waiting for XRDs to become established..."
	kubectl wait --for=condition=Established xrd/xwidgets.samples.xptracker.dev --timeout=30s
	kubectl wait --for=condition=Established xrd/xgadgets.samples.xptracker.dev --timeout=30s
	@echo "Waiting for function-patch-and-transform to become healthy..."
	kubectl wait --for=condition=Healthy function/function-patch-and-transform --timeout=120s
	kubectl apply -f hack/samples/compositions.yaml
	kubectl apply -f hack/samples/claims.yaml
	@echo "Sample resources applied. Claims should become Ready within a few seconds."

samples-delete: ## Delete sample resources from the cluster
	-kubectl delete -f hack/samples/claims.yaml
	-kubectl delete -f hack/samples/compositions.yaml
	-kubectl delete -f hack/samples/xrds.yaml
	-kubectl delete -f hack/samples/functions.yaml
	-kubectl delete -f hack/samples/namespaces.yaml

# --- Namespace ConfigMaps ---

namespace-config-apply: ## Apply sample per-namespace ConfigMaps for dev
	kubectl apply -f hack/samples/namespace-configmaps.yaml

namespace-config-delete: ## Delete sample per-namespace ConfigMaps
	-kubectl delete -f hack/samples/namespace-configmaps.yaml

# --- Kindplane ---

kindplane-up: ## Create kindplane cluster with Crossplane and Prometheus
	kindplane up

kindplane-down: ## Tear down the kindplane cluster
	kindplane down

kindplane-status: ## Show kindplane cluster status
	kindplane status

dev: kindplane-up samples-apply namespace-config-apply ## Bootstrap full dev environment (cluster + samples + namespace configs)
	@echo ""
	@echo "Dev environment ready. Start the exporter with:"
	@echo ""
	@echo "  make run-local              # central GVRs via env vars"
	@echo "  make run-local-namespace    # namespace ConfigMaps only (no central GVRs)"
	@echo ""
	@echo "Grafana: http://localhost:30300 (admin/admin)"
	@echo "Metrics: curl localhost:8080/metrics"
	@echo "Bookkeeping: curl localhost:8080/bookkeeping"

dev-down: samples-delete namespace-config-delete kindplane-down ## Tear down full dev environment

logs: ## Tail exporter logs (in-cluster deployment)
	kubectl logs -n crossplane-system deploy/crossplane-metrics-exporter -f

# --- Docs ---

DOCS_PORT ?= 8000

docs-serve: ## Serve MkDocs site locally via container (http://localhost:8000)
	docker run --rm -it -p $(DOCS_PORT):8000 -v $(CURDIR):/docs squidfunk/mkdocs-material

# --- Help ---

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*##' $(MAKEFILE_LIST) | sort | \
		awk 'BEGIN {FS = ":.*## "}; {printf "  \033[36m%-24s\033[0m %s\n", $$1, $$2}'
