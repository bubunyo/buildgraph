package sideeffect

// Registered is set to true by init, simulating a side-effect registration
// (e.g. a database driver, codec, or plugin).
var Registered bool
var Done bool

func init() {
	Registered = true
}

func init() {
	_ = 2
	Done = true
}
