NAME := taildog

default: test

test:
	go test ./...

build:
	go build -o $(NAME)

release:
	./scripts/release.sh

vet:
	@echo "go vet ."
	@go vet $$(go list ./... | grep -v vendor/) ; if [ $$? -eq 1 ]; then \
		echo ""; \
		echo "Vet found suspicious constructs. Please check the reported constructs"; \
		echo "and fix them if necessary before submitting the code for review."; \
		exit 1; \
	fi

fmt:
	gofmt -w .

.PHONY: test build release vet fmt
