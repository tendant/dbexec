# Variables
BINARY_NAME=dbexec
CMD_PATH=cmd/dbexec
BUILD_DIR=bin
VERSION ?= $(shell git describe --tags --always --dirty)

# Default target
all: build

build:
	go build -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_PATH)

clean:
	rm -rf $(BUILD_DIR)

run:
	./$(BUILD_DIR)/$(BINARY_NAME) run --config-path=samples/bootstrap.yaml

build-static:
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o $(BUILD_DIR)/$(BINARY_NAME) ./$(CMD_PATH)

# Updated buildx-based Docker image build
docker-build:
	docker buildx build \
		--platform linux/amd64,linux/arm64 \
		--push \
		--tag wang/dbexec:$(VERSION) \
		--tag wang/dbexec:latest \
		.

docker-run:
	docker run --rm \
		-e DATABASE_URL=$(DATABASE_URL) \
		-e TEST_USER_PASSWORD=$(TEST_USER_PASSWORD) \
		-v $(PWD)/samples/bootstrap.yaml:/bootstrap.yaml \
		$(BINARY_NAME):$(VERSION) run --config-path=/bootstrap.yaml

install:
	go install github.com/tendant/dbexec/$(CMD_PATH)@latest