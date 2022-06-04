all: lookup/lookup preload/preload.so

lookup/lookup: lookup/lookup.go
	go build -o lookup/lookup lookup/lookup.go

preload/preload.so: preload/preload.go
	go build -buildmode=c-shared -o preload/preload.so preload/preload.go	

NAME ?= "example.com"
test: all
	@echo Looking up $(NAME)...
	@LD_PRELOAD=$(PWD)/preload/preload.so $(PWD)/lookup/lookup $(NAME)

clean:
	@rm -fv lookup/lookup preload/preload.h preload/preload.so
	@rm -rfv _obj preload/_obj

.PHONY: clean test

