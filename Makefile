.PHONY: build run check clean

build:
	go build -trimpath -buildvcs=false -ldflags="-s -w -buildid=" -o bin/arcana-world ./cmd/arcana-world

run:
	go run ./cmd/arcana-world

check:
	go test -race ./...
	go vet ./...

clean:
	rm -f bin/arcana-world
