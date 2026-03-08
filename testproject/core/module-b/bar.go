package module_b

import "fmt"

func Save(data string) error {
	fmt.Println("Saving:", data)
	return nil
}

func Delete(id string) error {
	fmt.Println("Deleting:", id)
	return nil
}
