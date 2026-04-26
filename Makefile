IMAGE_REPO ?= ghcr.io/ursweiss/simple-local-path-provisioner
IMAGE_TAG  ?= latest

.PHONY: build docker-build helm-lint tidy test lint vuln pre-commit check

build:
	go build -o bin/driver ./cmd/driver

tidy:
	go mod tidy

docker-build:
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .

helm-lint:
	helm lint deploy/helm/simple-local-path-provisioner

test:
	go test ./...

lint:
	golangci-lint run ./...

vuln:
	govulncheck ./...

pre-commit:
	pre-commit run --all-files

check: lint test vuln helm-lint pre-commit
