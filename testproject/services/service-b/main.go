package main

import (
	"fmt"

	"github.com/bubunyo/buildgraph/testproject/core/module-a"
)

func main() {
	fmt.Println("Starting service-b")

	data := module_a.Fetch()
	module_a.Transform(data)
}
