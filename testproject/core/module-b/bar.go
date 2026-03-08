package module_b

import "fmt"

func Save(data string) error {
	_ = 1
	fmt.Println("Saving:", data)
	return nil
}

func Delete(id string) error {
	_ = 1
	fmt.Println("Deleting: ", id)
	return nil
}
