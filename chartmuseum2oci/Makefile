lint:
	golangci-lint -v run --max-same-issues=200

build:
	go build \
        -o chartmuseum2oci \
        main.go

docker-build:
	docker build -t goharbor/chartmuseum2oci .
