package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"syscall"

	"golang.org/x/crypto/bcrypt"
	"golang.org/x/term"
)

func do() error {
	fmt.Print("Password: ")
	pass, err := term.ReadPassword(int(syscall.Stdin))
	if err != nil {
		return fmt.Errorf("while reading password: %w", err)
	}

	hash, err := bcrypt.GenerateFromPassword(pass, 0)
	if err != nil {
		return fmt.Errorf("while hashing password: %w", err)
	}

	fmt.Println(string(hash))
	return nil
}

func main() {
	flag.Parse()

	if err := do(); err != nil {
		log.Printf("Error: %v", err)
		os.Exit(1)
	}
}
