.PHONY: clean lint security critic test install build release build-and-push-images delete-tag update-pkg-cache

clean:
	rm -rf build

lint:
	$(GOPATH)/bin/golangci-lint run --timeout 5m ./..

security:
	$(GOPATH)/bin/gosec ./..

critic:
	$(GOPATH)/bin/gocritic check -enableAll ./...

test: clean init security critics
	mkdir ./tests
	go test -coverprofile=./tests/coverage.out ./...
	go tool cover -func=./tests/coverage.out
	rm -rf ./tests


install: test
	CGO_ENABLED=0 go build -ldflags="-s -w" -o $(GOPATH)/bin/wetlog ./main.go

build: test
	$(GOPATH)/bin/goreleaser --snapshot --skip-publish --rm-dist

release: test
	git tag -a v$(VERSION) -m "$(VERSION)"
	$(GOPATH)/bin/goreleaser --snapshot --skip-publish --rm-dist

build-and-push-images: test
	docker build -t docker.io/kenjords/wetlog:latest .
	docker push docker.io/kenjords/wetlog:latest
	docker build -t docker.io/kenjords/wetlog:$(VERSION) .
	docker push docker.io/kenjords/wetlog:$(VERSION)
	docker image rm docker.io/kenjords/wetlog:$(VERSION)

delete-tag:
	git tag --delete v$(VERSION)
	docker image rm docker.io/kenjords/wetlog:latest
	docker image rm docker.io/kenjords/wetlog:$(VERSION)