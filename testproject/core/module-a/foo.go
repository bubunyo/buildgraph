package module_a

import "fmt"

func Process(input string) string {
	result := Transform(input)
	fmt.Println("Syntetic change")
	return result
}

func Fetch() string {
	return "data"
}

func Transform(data string) string {
	return fmt.Sprintf("processed and transformed: %s", data)
}
