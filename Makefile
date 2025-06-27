CURRENT_DIR=$(shell pwd)
APP=$(shell basename ${CURRENT_DIR})
APP_CMD_DIR=${CURRENT_DIR}/cmd

last-tag: ## Get last git tag
	git describe --tags

# Run all linters
lint:
	@echo "Running golangci-lint..."
	@golangci-lint run ./...

# Run linters with verbose output
lint-verbose:
	@echo "Running golangci-lint with verbose output..."
	@golangci-lint run -v ./...

# Run linters and show only new issues
lint-new:
	@echo "Running golangci-lint for new issues only..."
	@golangci-lint run --new-from-rev=HEAD~1 ./...

# Run linters with auto-fix
lint-fix:
	@echo "Running golangci-lint with auto-fix..."
	@golangci-lint run --fix ./...

# Run specific linter
lint-specific:
	@echo "Running specific linter: $(LINTER)"
	@golangci-lint run --disable-all --enable $(LINTER) ./...

# Show linter config
lint-config:
	@echo "Showing effective linter configuration..."
	@golangci-lint config

# List all available linters
lint-list:
	@echo "Available linters:"
	@golangci-lint linters

# Run linters with cache clean
lint-clean:
	@echo "Cleaning linter cache..."
	@golangci-lint cache clean
	@echo "Running golangci-lint..."
	@golangci-lint run ./...

# Run fast linters only
lint-fast:
	@echo "Running fast linters only..."
	@golangci-lint run --fast ./...

# Generate lint report in different formats
lint-report-json:
	@echo "Generating JSON lint report..."
	@golangci-lint run --out-format json > lint-report.json

lint-report-html:
	@echo "Generating HTML lint report..."
	@golangci-lint run --out-format html > lint-report.html

lint-report-junit:
	@echo "Generating JUnit XML lint report..."
	@golangci-lint run --out-format junit-xml > lint-report.xml

# CI-specific lint command (stricter)
lint-ci:
	@echo "Running golangci-lint for CI..."
	@golangci-lint run --timeout=10m --config=.golangci.yaml ./...
