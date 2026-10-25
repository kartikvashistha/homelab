set shell := ["bash", "-euo", "pipefail", "-c"]

project := "pulumi/projects/k8s"
stack   := "dreadnought"

# Each of these is its own Go module (see go.work); `./...` only matches
# packages inside the module you're standing in, so we loop.
modules := "pulumi/components/k8s pulumi/projects/k8s"

default:
    @just --list

fmt:
    gofmt -w pulumi

# Extra flags pass through: `just vet -v`, `just lint --fix`
vet *args:
    for m in {{modules}}; do echo "==> vet $m"; (cd "$m" && go vet {{args}} ./...); done

lint *args:
    for m in {{modules}}; do echo "==> lint $m"; (cd "$m" && golangci-lint run {{args}} ./...); done

tidy:
    for m in {{modules}}; do echo "==> tidy $m"; (cd "$m" && go mod tidy); done

# Show what would change, with a diff.
preview:
    cd {{project}} && pulumi preview --stack {{stack}} --diff

up:
    cd {{project}} && pulumi up --stack {{stack}} --skip-preview
