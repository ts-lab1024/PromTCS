
all: query storage multi_storage

query:
	cd service/query && go build -o ../../bin/query query.go

storage:
	cd service/storage && go build -o ../../bin/storage storage.go

multi_storage:
	cd service/multi_storage && go build -o ../../bin/multi_storage multi_storage.go

clean:
	rm -f bin/query bin/storage

.PHONY: all query storage clean