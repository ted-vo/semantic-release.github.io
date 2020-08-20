package main

import (
	"fmt"
	"os"
)

func checkError(err error) {
	if err != nil {
		panic(err)
	}
}

func main() {
	dest := "./plugin-index/api/v1/"
	fmt.Printf("creating %s\n", dest)
	checkError(os.RemoveAll("./plugin-index"))
	checkError(os.MkdirAll(dest, 0755))
}
