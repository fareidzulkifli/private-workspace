package main

import (
	"fmt"
	"os"

	"private-workspace/internal/auth"
)

func main() {
	password := os.Getenv("ADMIN_PASSWORD")
	if password == "" {
		fmt.Fprintln(os.Stderr, "ADMIN_PASSWORD is required")
		os.Exit(1)
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	fmt.Println(hash)
}
