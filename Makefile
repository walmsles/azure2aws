BINARY_NAME=azure2aws
VERSION=$(shell cat version.txt)

.PHONY: build clean sign install bump-patch bump-minor bump-major

build:
	go build -ldflags "-X main.version=$(VERSION)" -o $(BINARY_NAME) main.go

sign: build
	codesign --force --sign - ./$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)

install: sign
	cp $(BINARY_NAME) /usr/local/bin/

bump-patch:
	./bump-version.sh patch

bump-minor:
	./bump-version.sh minor

bump-major:
	./bump-version.sh major

all: sign
