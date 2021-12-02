//go:build js && wasm

package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"syscall/js"

	"github.com/crhntr/window"
)

func main() {
	_, err := window.LoadTemplates(nil, "template")
	if err != nil {
		fmt.Println("failed to load template", err)
		return
	}

	doAll(fetchGoVersion, fetchGoEnv)

	window.Document.QuerySelector("button#run").AddEventListenerFunc("click", func(event window.Event) {
		go run()
	})

	select {}
}

func run() {
	code := window.Document.QuerySelector("textarea#code").Get("value").String()

	req, err := http.NewRequest(http.MethodPost, "/go/run", strings.NewReader(code))
	if err != nil {
		fmt.Println("failed to create request", err)
		return
	}
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		fmt.Println("failed to do request", err)
		return
	}
	defer func() {
		_ = res.Body.Close()
	}()

	if res.StatusCode != http.StatusOK {
		buf, err := io.ReadAll(res.Body)
		if err != nil {
			fmt.Println("failed to read response", err)
			return
		}
		window.Document.QuerySelector("ul#errors").Append(window.Document.NewElement("%s", string(buf)))
		return
	}

	_, params, err := mime.ParseMediaType(res.Header.Get("Content-Type"))
	if err != nil {
		fmt.Println("failed to parse response mimetype", err)
		return
	}
	r := multipart.NewReader(res.Body, params["boundary"])

	for {
		part, err := r.NextRawPart()
		if err != nil {
			if err != io.EOF {
				fmt.Println("failed to read response part", err)
				return
			}
			break
		}

		switch part.FormName() {
		case "stdout":
			buf, err := io.ReadAll(part)
			if err != nil {
				fmt.Println("failed to read response stdout", err)
				return
			}
			window.Document.QuerySelector("pre#stdout").SetInnerText(string(buf))
		case "stderr":
			buf, err := io.ReadAll(part)
			if err != nil {
				fmt.Println("failed to read response stdout", err)
				return
			}
			window.Document.QuerySelector("pre#stderr").SetInnerText(string(buf))
		case "output":
			buf, err := ioutil.ReadAll(part)
			if err != nil {
				fmt.Println("failed to read response output", err)
				return
			}
			g := js.Global()
			uint8Array := g.Get("Uint8Array").New(len(buf))
			_ = js.CopyBytesToJS(uint8Array, buf)

			done := make(chan struct{})
			wasmExec := g.Get("Go").New()
			success := js.FuncOf(func(this js.Value, args []js.Value) interface{} {
				defer close(done)
				result := args[0]
				wasmExec.Call("run", result.Get("instance"))
				return nil
			})

			g.Get("WebAssembly").Call(
				"instantiate", uint8Array.Get("buffer"), wasmExec.Get("importObject"),
			).Call("then", success)

			<-done
		}
	}
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
