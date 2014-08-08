package main

import (
	"log"

	"tools/veyron/impl"
)

func main() {
	root, err := impl.Root()
	if err != nil {
		log.Fatalf("%v", err.Error())
	}
	root.Main()
}
