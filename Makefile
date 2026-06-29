.PHONY: help dev build clean

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'

dev: ## Run development server
	set -a; source .env; set +a; go run . -f etc/portfolio-api.yaml

build: ## Build the binary
	go build -o portfolio-backend .

clean: ## Remove built binary
	rm -f portfolio-backend
