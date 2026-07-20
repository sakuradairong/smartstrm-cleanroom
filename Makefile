.PHONY: fmt tidy vet test check build docker-build

fmt:
	find cmd internal -type f -name '*.go' -print0 | xargs -0 gofmt -w

tidy:
	go mod tidy

vet:
	go vet ./...

test:
	go test -race ./...

check: vet test
	python3 -m json.tool config.example.json >/dev/null
	python3 scripts/check_markdown_links.py
	docker compose config -q
	git diff --check

build:
	go build -trimpath -o smartstrm ./cmd/smartstrm

docker-build:
	docker compose build smartstrm
