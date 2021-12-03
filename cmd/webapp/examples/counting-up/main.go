package main

import (
	"fmt"
	"syscall/js"
	"time"
)

func main() {
	for i := 0; i < 8; i++ {
		js.Global().Get("document").Get("body").Set("innerText", fmt.Sprint(i))
		time.Sleep(time.Second / 2)
	}
}
