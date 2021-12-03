//go:build js && wasm

package main

import (
	_ "embed"
	"fmt"
	"html/template"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"strings"
	"sync"
	"syscall/js"
	"time"

	"github.com/crhntr/window"
)

//go:embed index.gohtml
var indexHTML string

func main() {
	window.SetTemplates(template.Must(template.New("").Parse(indexHTML)))

	doAll(fetchGoVersion, fetchGoEnv)

	window.Document.QuerySelector("button#run").AddEventListenerFunc("click", func(event window.Event) {
		go handleRun()
	})

	select {}
}

func handleRun() {
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
			runWASM(buf)
		}
	}
}

func runWASM(buf []byte) {
	runBox, err := window.Document.NewElementFromTemplate("run", struct{}{})
	if err != nil {
		panic(err)
	}
	frame := runBox.QuerySelector("iframe.run")
	var (
		closeLoadHandler    = func() {}
		closeFrameHandler   = func() {}
		closeMessageHandler = func() {}
	)
	closeHandler := func() {
		closeLoadHandler()
		closeFrameHandler()
		closeMessageHandler()
	}
	closeLoadHandler = frame.AddEventListenerFunc("load", func(event window.Event) {
		defer closeLoadHandler()
		message := window.Get("Object").New()
		message.Set("name", "binary")
		array := window.Get("Uint8ClampedArray").New(len(buf))
		_ = js.CopyBytesToJS(array, buf)
		message.Set("binary", array)
		frame.Get("contentWindow").Call("postMessage", message, window.Document.Get("location").Get("origin"))
	})
	closeMessageHandler = window.AddEventListenerFunc("message", func(event window.Event) {
		defer closeMessageHandler()

		d := event.Get("data")
		messageName := d.Get("name").String()

		switch messageName {
		case "exit":
			processEnd := struct {
				ExitCode int
				Duration time.Duration
			}{
				ExitCode: d.Get("exitCode").Int(),
				Duration: time.Duration(d.Get("duration").Int()) * time.Millisecond,
			}
			el, err := window.Document.NewElementFromTemplate("exit-status", processEnd)
			if err != nil {
				panic(err)
				return
			}
			runBox.Append(el)
		}
	})
	closeFrameHandler = runBox.QuerySelector("button.close").AddEventListenerFunc("click", func(event window.Event) {
		defer closeHandler()
		window.Console.Log("HELLO")
		frame.Get("contentWindow").Call("close")
		runBox.Closest("div.run").Remove()
	})

	window.Document.Body().Append(runBox)
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
