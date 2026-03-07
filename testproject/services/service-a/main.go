package main

import (
	"fmt"

	"github.com/bubunyo/buildgraph/testproject/core/module-a"
	"github.com/bubunyo/buildgraph/testproject/core/module-b"
)

func main() {
	fmt.Println("Starting service-a")

	result := module_a.Process("test")
	module_b.Save(result)
}
