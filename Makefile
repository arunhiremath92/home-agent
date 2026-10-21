.PHONY: all test check build-agent-server build-raspberry-pi run stop

all: build-agent-server

test:
	cd agent-server && go test ./...
	python3 -m unittest discover -s agent-server/scripts -p 'test_*.py' -v

check:
	@unformatted="$$(gofmt -l $$(find agent-server -type f -name '*.go'))"; \
	if [ -n "$$unformatted" ]; then \
		printf 'Go files need formatting:\n%s\n' "$$unformatted"; \
		exit 1; \
	fi
	cd agent-server && go vet ./...
	$(MAKE) test

build-agent-server:
	docker build -t home-agent:desktop --target desktop -f deployment/Dockerfile .

build-raspberry-pi:
	docker buildx build --platform linux/arm64 --load --target raspberry-pi -t home-agent:raspberry-pi -f deployment/Dockerfile .

run:
	docker compose -f deployment/compose.yaml up --build

stop:
	docker compose -f deployment/compose.yaml down
