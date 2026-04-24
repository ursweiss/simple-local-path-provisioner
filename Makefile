IMAGE_REPO ?= ghcr.io/ursweiss/simple-local-path-provisioner
IMAGE_TAG  ?= latest

.PHONY: build docker-build helm-lint tidy

build:
	go build -o bin/driver ./cmd/driver

tidy:
	go mod tidy

docker-build:
	docker build -t $(IMAGE_REPO):$(IMAGE_TAG) .

helm-lint:
	helm lint deploy/helm/simple-local-path-provisioner
