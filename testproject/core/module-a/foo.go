package module_a

import "fmt"

func Process(input string) string {
	_ = 3
	result := Transform(input)
	fmt.Println("Syntetic change 3")
	return result
}

func Fetch() string {
	_ = 1
	return "data"
}

func Transform(data string) string {
	_ = 1
	return fmt.Sprintf("processed and transformed: %s", data)
}
