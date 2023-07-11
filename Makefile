SHELL=/usr/bin/env bash

GOCC?=go
BINS:=

example:
	$(GOCC) build $(GOFLAGS) -o example ./
.PHONY: example
BINS+=example

clean:
	rm -rf $(BINS)
.PHONY: clean