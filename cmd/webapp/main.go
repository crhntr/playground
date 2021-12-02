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

func fetchGoVersion() {
	replaceTextWithResponseBody(
		"/go/version",
		"details#go-version pre",
	)
}

func fetchGoEnv() {
	replaceTextWithResponseBody(
		"/go/env",
		"details#go-env pre",
	)
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

func replaceTextWithResponseBody(urlPath, elementQuery string) {
	res, err := http.Get(urlPath)
	if err != nil {
		fmt.Printf("failed to fetch %s: %s", urlPath, err)
		return
	}
	defer func() {
		_ = res.Body.Close()
	}()
	body, err := io.ReadAll(res.Body)
	if err != nil {
		fmt.Println("failed to read response", err)
		return
	}
	window.Document.QuerySelector(elementQuery).SetInnerText(string(body))
}
