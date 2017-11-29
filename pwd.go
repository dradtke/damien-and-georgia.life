package main

import (
	"fmt"
	"io/ioutil"
	"os"

	"golang.org/x/crypto/bcrypt"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Printf("usage: go run pwd.go <password>")
	}

	plain := os.Args[1]
	hash, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		panic(err)
	}

	if err := ioutil.WriteFile("password", hash, 0600); err != nil {
		panic(err)
	}

	fmt.Println("password saved")
}
