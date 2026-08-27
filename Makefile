.PHONY: build build-windows test race vet check clean

build:
	go build -trimpath -o paramintel ./cmd/paramintel

build-windows:
	GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -trimpath -o paramintel-windows-amd64.exe ./cmd/paramintel

test:
	go test ./...

race:
	go test -race ./...

vet:
	go vet ./...

check: test race vet build

clean:
	rm -f paramintel paramintel.exe paramintel-* coverage.out
