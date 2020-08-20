package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"path"
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
	plugJson := path.Join(dest, "plugins.json")
	fmt.Printf("creating %s\n", plugJson)
	checkError(ioutil.WriteFile(plugJson, []byte("{}"), 0755))
}
