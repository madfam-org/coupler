.PHONY: gateway-test gateway-run mcp-check

gateway-test:
	cd apps/gateway && go test ./...

gateway-run:
	cd apps/gateway && COUPLER_CONNECTORS_DIR=../../connectors go run ./cmd/gateway

mcp-check:
	python3 -m py_compile packages/mcp-server/server.py
