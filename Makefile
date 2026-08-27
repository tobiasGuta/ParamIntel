.PHONY: build test race vet check clean

build:
	go build -o paramintel ./cmd/paramintel

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check: test race vet build

clean:
	rm -f paramintel coverage.out
