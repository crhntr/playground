package main

import (
	"fmt"
	"net/http"
)

func main() {
	// this will panic because the iframe sandbox does not have "allow-same-origin"
	req, err := http.Get("http://localhost:8080")
	if err != nil {
		panic(err)
	}
	fmt.Println(req.StatusCode)
}
