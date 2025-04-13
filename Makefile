.PHONY: build test lint clean run pre-commit all help

BINARY_NAME=websocket-exporter
GO=go
GOLANGCI_LINT=golangci-lint

all: clean build test

help:
	@echo "Available commands:"
	@echo "  make build      - Build the websocket-exporter binary"
	@echo "  make test       - Run tests"
	@echo "  make lint       - Run linters"
	@echo "  make clean      - Remove build artifacts"
	@echo "  make run        - Build and run the exporter"
	@echo "  make pre-commit - Run pre-commit checks"
	@echo "  make all        - Run clean, build, test"
	@echo "  make help       - Display this help"

build:
	$(GO) build -o $(BINARY_NAME) -v

test:
	$(GO) test -v -race -coverprofile=coverage.txt -covermode=atomic ./...

lint:
	$(GOLANGCI_LINT) run

clean:
	$(GO) clean
	rm -f $(BINARY_NAME)
	rm -f coverage.txt coverage.html coverage.out

run: build
	./$(BINARY_NAME)

pre-commit:
	pre-commit run --all-files
