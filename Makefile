.PHONY: fmt test race vet build image catalog-test
fmt:
	test -z "$$(gofmt -l .)"
test:
	go test ./...
race:
	go test -race ./...
vet:
	go vet ./...
build:
	CGO_ENABLED=0 go build ./cmd/recipebot
image:
	docker build -t telegram-recipe-bot:dev .


catalog-test:
	python3 -m unittest discover -s tools/catalog -p "test_*.py"
