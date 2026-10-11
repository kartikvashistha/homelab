set shell := ["bash", "-euo", "pipefail", "-c"]

project := "pulumi/projects/k8s"
stack   := "dreadnought"

default:
    @just --list

fmt:
    gofmt -w pulumi

vet:
    go vet ./...

lint:
    golangci-lint run ./...

# Show what would change, with a diff.
preview:
    cd {{project}} && pulumi preview --stack {{stack}} --diff

up:
    cd {{project}} && pulumi up --stack {{stack}}
