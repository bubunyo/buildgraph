package main

import (
	"fmt"

	"github.com/bubunyo/buildgraph/testproject/core/collision"
	module_a "github.com/bubunyo/buildgraph/testproject/core/module-a"
	module_b "github.com/bubunyo/buildgraph/testproject/core/module-b"
)

func main() {
	fmt.Println("Starting service-a")

	result := module_a.Process("test")
	module_b.Save(result)

	// Exercise both collision types so they appear in the call graph.
	a := &collision.A{}
	b := &collision.B{}
	a.Run()
	b.Run()
}
