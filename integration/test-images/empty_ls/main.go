package main

import (
	"fmt"
	"io/ioutil"
)

func main() {
	files, err := ioutil.ReadDir("/")
	mustNot(err)

	for _, entry := range files {
		fmt.Println(entry.Name())
	}
}

func mustNot(err error) {
	if err != nil {
		panic(err)
	}
}
