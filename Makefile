.PHONY: build
build:
	go build -o bin/kubectl-node-maintain cmd/kubectl-node-maintain/main.go

.PHONY: install
install: build
	cp bin/kubectl-node-maintain $(GOPATH)/bin/

.PHONY: test
test:
	go test ./...

.PHONY: clean
clean:
	rm -rf bin/ 