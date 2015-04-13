package main

import (
	"fmt"
	"os"
)

func main() {
	fmt.Fprint(os.Stdout, "hello from stdout")
	fmt.Fprint(os.Stderr, "hello from stderr")
}
