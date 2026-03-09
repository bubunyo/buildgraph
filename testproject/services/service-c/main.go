package main

import (
	"fmt"

	module_a "github.com/bubunyo/buildgraph/testproject/core/module-a"
)

func main() {
	fmt.Println("Starting service-c")

	data := module_a.Fetch()
	fmt.Println(module_a.Transform(data))
}
