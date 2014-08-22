OPTIMIZATION?=-O0
DEBUG?=-g -ggdb -rdynamic

all: nstar

clean:
		rm -f *.o nstar

.PHONY: all clean

nstar: nstar.o
		$(CC) -o $@ $^

%.o: %.c
		$(CC) -c -Wall -D_GNU_SOURCE $(OPTIMIZATION) $(DEBUG) $(CFLAGS) $<

