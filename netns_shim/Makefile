CC=gcc
OBJ = netns_shim.o
CFLAGS ?= -g

%.o: %.c
	$(CC) -c -o $@ $< $(CFLAGS)


netns_shim: $(OBJ)
	$(CC) -o $@ $^ $(CFLAGS) -lev

.PHONY: all
all: netns_shim

.PHONY: clean
clean:
	@rm -fv $(OBJ) netns_shim
