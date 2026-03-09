package main

import (
	"fmt"

	module_a "github.com/bubunyo/buildgraph/testproject/core/module-a"
)

func main() {
	fmt.Println("Running tool-a")

	// tool-a uses the same shared core as services, so a change to module-a
	// would propagate here too — but tool-a must never appear in
	// ServicesToBuild when serviceDirs is restricted to ["services"].
	result := module_a.Process("tool-input")
	fmt.Println(result)
}
