//go:build js && wasm

package main

import (
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/crhntr/window"
)

func main() {
	_, err := window.LoadTemplates(nil, "template")
	if err != nil {
		fmt.Println("failed to load template", err)
		return
	}

	doAll(fetchGoVersion, fetchGoEnv)
}

func doAll(fns ...func()) {
	wg := sync.WaitGroup{}
	wg.Add(len(fns))
	for _, fn := range fns {
		go func(f func()) {
			defer wg.Done()
			f()
		}(fn)
	}
	wg.Wait()
}

func fetchGoVersion() {
	res, err := http.Get("/go/version")
	if err != nil {
		fmt.Println("failed to fetch version", err)
		return
	}
	defer func() {
		_ = res.Body.Close()
	}()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("failed to read version", err)
		return
	}
	window.Document.QuerySelector("details#go-version pre").SetInnerText(string(body))
}

func fetchGoEnv() {
	res, err := http.Get("/go/env")
	if err != nil {
		fmt.Println("failed to fetch version", err)
		return
	}
	defer func() {
		_ = res.Body.Close()
	}()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("failed to read version", err)
		return
	}
	window.Document.QuerySelector("details#go-env pre").SetInnerText(string(body))
}
