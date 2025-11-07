BINARY_NAME=azure2aws

.PHONY: build clean sign install

build:
	go build -o $(BINARY_NAME) main.go

sign: build
	codesign --force --sign - ./$(BINARY_NAME)

clean:
	rm -f $(BINARY_NAME)

install: sign
	cp $(BINARY_NAME) /usr/local/bin/

all: sign
