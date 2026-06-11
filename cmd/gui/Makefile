.PHONY: build frontend dev clean install

# Build frontend assets then compile the GUI binary
build: frontend
	go build -o xbot-gui .

# Build frontend React app into frontend/dist/
frontend:
	cd frontend && npm install --silent && npx vite build --config vite.config.gui.ts

# Development mode: run directly (requires frontend build first)
dev: frontend
	go run .

# Clean build artifacts
clean:
	rm -rf frontend/dist frontend/node_modules xbot-gui

# Install to local PATH
install: build
	cp xbot-gui ~/go/bin/xbot-gui
