package main

import "fmt"

func main() {
	files := []string{"notes.md", "letter.eml"}
	for _, name := range files {
		fmt.Printf("Preview: %s\n", name)
	}
}
