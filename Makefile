BIN    := lira
CMD    := .
PREFIX := /usr/local

.PHONY: build run clean tidy install

build:
	go build -o $(BIN) $(CMD)

run: build
	./$(BIN)

tidy:
	go mod tidy

install: build
	install -m 755 $(BIN) $(PREFIX)/bin/$(BIN)

clean:
	rm -f $(BIN)
