.PHONY: build build-local test clean

build:
	GOOS=windows GOARCH=amd64 /opt/data/go/bin/go build -o search.exe ./cmd/search-app

build-local:
	/opt/data/go/bin/go build -o search ./cmd/search-app

test:
	/opt/data/go/bin/go test ./...

clean:
	rm -f search.exe search semantic-search.db
