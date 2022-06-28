package main

import (
	"fmt"
	"math/rand"
	"syscall/js"
	"time"
)

func main() {
	rand.Seed(time.Now().Unix())
	max := rand.Intn(20) + 5
	fmt.Printf("max: %d\n", max)

	body := js.Global().Get("document").Get("body")
	body.Set("innerHTML", `<ul></ul>`)

	ul := body.Call("querySelector", "ul")
	for i := 0; i < max; i++ {
		ul.Call("insertAdjacentHTML", "beforeEnd", fmt.Sprintf("<li>%d</li>", i))
		time.Sleep(time.Second)
	}
}
