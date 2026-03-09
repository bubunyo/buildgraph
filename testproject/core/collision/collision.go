// Package collision exists solely to test that funcKey correctly distinguishes
// methods of the same name on different receiver types within the same package.
package collision

// A is a type whose Run method must not collide with (*B).Run.
type A struct{}

// B is a type whose Run method must not collide with (*A).Run.
type B struct{}

// Run on A does nothing; it exists to create a same-name method collision scenario.
func (a *A) Run() {}

// Run on B does nothing; it exists to create a same-name method collision scenario.
func (b *B) Run() {}
