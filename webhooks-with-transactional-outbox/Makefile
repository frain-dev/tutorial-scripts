.PHONY: all clean build generate

all: generate build

generate:
	sqlc generate

build: generate
	mkdir -p bin
	go build -o bin/transactional-outbox main.go

clean:
	rm -rf bin/
	rm -f events.db 

init-db:
	sqlite3 events.db < schema.sql 