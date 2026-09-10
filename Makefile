.PHONY: fmt test race vet build image
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

