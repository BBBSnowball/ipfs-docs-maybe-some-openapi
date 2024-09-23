all: install
install:
	GO111MODULE=on go install ./http-api-docs

generate-openapi openapi.yaml:
	go run ./http-api-openapi/main.go >openapi.yaml

.PRECIOUS: openapi.yaml

%.sorted.yaml: %.yaml
	yq -y --sort-keys <$< >$@

test-openapi: generate-openapi ipfs-api.sorted.yaml openapi.sorted.yaml
