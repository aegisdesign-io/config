.PHONY: help
help: ## Display valid Makefile targets.
	@echo 'targets:'
	@echo
	@grep -E '^[a-zA-Z_-]+:[[:space:]]*##[[:space:]]*.*$$' $(MAKEFILE_LIST) | sed -E 's/:[[:space:]]*##[[:space:]]*/#/' | column -s '#' -t

.PHONY: clean
clean: ## Clean up build artifacts
	@rm -f coverage.out coverage.html

.PHONY: update
update: ## Updates go module dependencies
	@go get -u
	@go mod tidy

.PHONY: test
test: ## Run unit tests
	@go test ./...

.PHONY: coverage
coverage: ## Run unit tests with coverage analysis
	@go test -covermode=atomic -coverprofile=coverage.out ./...
	@go tool cover -html=coverage.out -o coverage.html
	@echo 'open coverage.html for coverage heat map'
