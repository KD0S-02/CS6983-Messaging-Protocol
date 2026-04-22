# CS6983 Messaging Protocol Makefile
#
# Run all make commands from the repository root:
#
#   C:\Users\prana\OneDrive\Desktop\CS6983-Messaging-Protocol
#
# Common examples:
#
#   make help
#   make fmt
#   make test
#   make run
#   make api-put VALUE="hello world" KEY=demo
#   make api-get-gw2 KEY=demo
#   make git-submit MSG="Add Makefile workflow commands"
#
# Notes:
# - This Makefile uses .RECIPEPREFIX so commands start with ">" instead of tabs.
# - Runtime SSD block files under data/ should not be committed.

.DEFAULT_GOAL := help
.RECIPEPREFIX := >

HTTP ?= 127.0.0.1:18080
BASE ?= http://$(HTTP)

KEY ?= demo
VALUE ?= hello world

PKG ?= ./internal/gateway
TEST ?= TestEndToEndPutOnGateway1ReadableFromGateway2

MSG ?= Update project

GO_PACKAGES := ./...
FMT_DIRS := internal/proto internal/ssd internal/transport internal/gateway cmd/devcluster

.PHONY: help
help:
> @echo "CS6983 Messaging Protocol"
> @echo ""
> @echo "Setup / information:"
> @echo "  make version              Show Go and Git versions"
> @echo "  make status               Show git status"
> @echo "  make tidy                 Run go mod tidy"
> @echo ""
> @echo "Formatting / checks:"
> @echo "  make fmt                  Format Go files"
> @echo "  make vet                  Run go vet"
> @echo "  make compile              Compile all packages without running tests"
> @echo "  make check                Run fmt, vet, and test"
> @echo ""
> @echo "Tests:"
> @echo "  make test                 Run all tests"
> @echo "  make test-v               Run all tests with verbose output"
> @echo "  make test-race            Run all tests with race detector"
> @echo "  make test-proto           Run proto package tests"
> @echo "  make test-ssd             Run SSD package tests"
> @echo "  make test-transport       Run transport package tests"
> @echo "  make test-gateway         Run gateway package tests"
> @echo "  make test-devcluster      Run cmd/devcluster tests"
> @echo "  make test-one             Run one test with PKG=... TEST=..."
> @echo ""
> @echo "Coverage:"
> @echo "  make cover                Run tests with coverage summary"
> @echo "  make cover-profile        Generate coverage.out"
> @echo "  make cover-html           Open HTML coverage report"
> @echo ""
> @echo "Dev cluster:"
> @echo "  make run                  Start local dev cluster on $(BASE)"
> @echo "  make run-default          Start local dev cluster on default port 8080"
> @echo "  make run-3gw              Start local dev cluster with 3 gateways"
> @echo "  make run-latency          Start cluster with SSD latency and jitter"
> @echo "  make run-failures         Start cluster with injected SSD failures"
> @echo "  make run-altports         Start cluster with alternate SSD and peer ports"
> @echo ""
> @echo "API checks, run these in a second terminal while make run is active:"
> @echo "  make api-health           Check /healthz"
> @echo "  make api-put              PUT value through gateway 1"
> @echo "  make api-get-gw1          GET value through gateway 1"
> @echo "  make api-get-gw2          GET value through gateway 2"
> @echo "  make api-info             Show cluster info"
> @echo "  make api-demo             Run health, put, get-gw1, get-gw2, and info"
> @echo ""
> @echo "Port troubleshooting:"
> @echo "  make port-8080            Show processes using port 8080"
> @echo "  make port-18080           Show processes using port 18080"
> @echo ""
> @echo "Cleanup:"
> @echo "  make clean-data           Remove local data/ runtime files"
> @echo "  make clean-coverage       Remove coverage.out"
> @echo "  make clean                Remove data/ and coverage.out"
> @echo ""
> @echo "Git / GitHub:"
> @echo "  make git-status           Show git status"
> @echo "  make git-log              Show recent commits"
> @echo "  make git-ignore-runtime   Add data/ and coverage.out to .gitignore"
> @echo "  make git-add              Stage project files without data/"
> @echo "  make git-commit MSG=...   Commit staged files"
> @echo "  make git-pull             Pull with rebase from origin/main"
> @echo "  make git-push             Push to origin/main"
> @echo "  make git-submit MSG=...   Add, commit, pull --rebase, test, and push"
> @echo "  make git-rebase-abort     Abort an in-progress rebase"

# -------------------------------------------------------------------
# Setup / information
# -------------------------------------------------------------------

.PHONY: version status tidy
version:
> go version
> git --version

status: git-status

tidy:
> go mod tidy

# -------------------------------------------------------------------
# Formatting / checks
# -------------------------------------------------------------------

.PHONY: fmt vet compile check
fmt:
> gofmt -w $(FMT_DIRS)

vet:
> go vet $(GO_PACKAGES)

compile:
> go test $(GO_PACKAGES) -run TestDoesNotExist

check: fmt vet test

# -------------------------------------------------------------------
# Tests
# -------------------------------------------------------------------

.PHONY: test test-v test-race test-proto test-ssd test-transport test-gateway test-devcluster test-one
test:
> go test $(GO_PACKAGES)

test-v:
> go test $(GO_PACKAGES) -v

test-race:
> go test $(GO_PACKAGES) -race

test-proto:
> go test ./internal/proto -v

test-ssd:
> go test ./internal/ssd -v

test-transport:
> go test ./internal/transport -v

test-gateway:
> go test ./internal/gateway -v

test-devcluster:
> go test ./cmd/devcluster -v

test-one:
> go test $(PKG) -run $(TEST) -v

# -------------------------------------------------------------------
# Coverage
# -------------------------------------------------------------------

.PHONY: cover cover-profile cover-html
cover:
> go test $(GO_PACKAGES) -cover

cover-profile:
> go test $(GO_PACKAGES) -coverprofile=coverage.out

cover-html: cover-profile
> go tool cover -html=coverage.out

# -------------------------------------------------------------------
# Dev cluster
# -------------------------------------------------------------------

.PHONY: run run-default run-3gw run-latency run-failures run-altports
run:
> go run ./cmd/devcluster -http $(HTTP)

run-default:
> go run ./cmd/devcluster

run-3gw:
> go run ./cmd/devcluster -http $(HTTP) -gateways 3

run-latency:
> go run ./cmd/devcluster -http $(HTTP) -latency-ms 100 -jitter-ms 50

run-failures:
> go run ./cmd/devcluster -http $(HTTP) -fail-rate 0.1

run-altports:
> go run ./cmd/devcluster -http $(HTTP) -ssd-addrs 127.0.0.1:19100,127.0.0.1:19101,127.0.0.1:19102 -peer-addrs 127.0.0.1:19201,127.0.0.1:19202

# -------------------------------------------------------------------
# HTTP API checks
#
# Start the cluster first:
#
#   make run
#
# Then open a second terminal and run:
#
#   make api-demo
# -------------------------------------------------------------------

.PHONY: api-health api-put api-get-gw1 api-get-gw2 api-info api-demo
api-health:
> curl.exe $(BASE)/healthz

api-put:
> curl.exe -X PUT --data-binary "$(VALUE)" $(BASE)/gateways/1/kv/$(KEY)

api-get-gw1:
> curl.exe $(BASE)/gateways/1/kv/$(KEY)

api-get-gw2:
> curl.exe $(BASE)/gateways/2/kv/$(KEY)

api-info:
> curl.exe $(BASE)/gateways

api-demo: api-health api-put api-get-gw1 api-get-gw2 api-info

# -------------------------------------------------------------------
# Port troubleshooting
# -------------------------------------------------------------------

.PHONY: port-8080 port-18080 port-9100 port-9201
port-8080:
> netstat -ano | findstr :8080

port-18080:
> netstat -ano | findstr :18080

port-9100:
> netstat -ano | findstr :9100

port-9201:
> netstat -ano | findstr :9201

# -------------------------------------------------------------------
# Cleanup
# -------------------------------------------------------------------

.PHONY: clean-data clean-coverage clean
clean-data:
> powershell -NoProfile -Command "if (Test-Path data) { Remove-Item -Recurse -Force data }"

clean-coverage:
> powershell -NoProfile -Command "if (Test-Path coverage.out) { Remove-Item -Force coverage.out }"

clean: clean-data clean-coverage

# -------------------------------------------------------------------
# Git / GitHub workflow
# -------------------------------------------------------------------

.PHONY: git-status git-log git-ignore-runtime git-add git-commit git-pull git-push git-submit git-rebase-abort
git-status:
> git status

git-log:
> git log --oneline -5

git-ignore-runtime:
> powershell -NoProfile -Command "if (-not (Test-Path .gitignore)) { New-Item -ItemType File .gitignore | Out-Null }; if (-not (Select-String -Path .gitignore -Pattern '^data/$$' -Quiet)) { Add-Content .gitignore 'data/' }; if (-not (Select-String -Path .gitignore -Pattern '^coverage.out$$' -Quiet)) { Add-Content .gitignore 'coverage.out' }"

git-add: git-ignore-runtime
> git add README.md Makefile .gitignore cmd internal

git-commit:
> git commit -m "$(MSG)"

git-pull:
> git pull --rebase origin main

git-push:
> git push origin main

git-submit: git-add git-commit git-pull test git-push

git-rebase-abort:
> git rebase --abort
