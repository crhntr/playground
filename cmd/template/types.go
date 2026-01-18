package main

type Lock struct {
	Key  string `json:"key"`
	Open bool   `json:"open"`
}

type Basket struct {
	Fruits []string
}
