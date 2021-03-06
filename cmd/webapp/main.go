//go:build js && wasm

package main

import (
	"embed"
	"fmt"
	"io"
	"io/ioutil"
	"mime"
	"mime/multipart"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall/js"
	"time"

	"github.com/crhntr/window"
	"github.com/crhntr/window/browser"
	"github.com/crhntr/window/dom"

	"github.com/crhntr/playground"
)

var (
	//go:embed run_head.html
	runHTMLHEAD string

	//go:embed assets/wasm_exec.js
	wasmExec string

	//go:embed examples
	examplesFS embed.FS
)

func main() {
	exampleNames := playground.ListFileNames(examplesFS, "examples")
	selectExample := window.Document.QuerySelector("select#select-example").(browser.Element)
	for _, name := range exampleNames {
		option := window.Document.CreateElement("option")
		option.SetAttribute("value", name)
		option.AppendChild(window.Document.CreateTextNode(name))
		selectExample.Append(option)
	}
	updateExampleCode(exampleNames[0])

	exampleChangeHandler := browser.NewEventListenerFunc(updateExampleCodeHandler)
	defer exampleChangeHandler.Release()
	selectExample.AddEventListener("change", exampleChangeHandler)

	runButton := window.Document.QuerySelector("button#run").(browser.Element)
	var runCount int64
	runBtnClickHandler := browser.NewEventListenerFunc(func(event browser.Event) {
		atomic.AddInt64(&runCount, 1)
		go handleRun(int(runCount))
	})
	defer runBtnClickHandler.Release()
	runButton.AddEventListener("click", runBtnClickHandler)

	editorEl := window.Document.QuerySelector("#editor")
	editorEl.RemoveAttribute("hidden")
	defer editorEl.SetAttribute("hidden", "")

	messageHandler := browser.NewEventListenerFunc(func(event browser.Event) {
		d := js.Value(event).Get("data")
		messageName := d.Get("name").String()

		runNode := window.Document.QuerySelector(fmt.Sprintf(`[data-run-id="%d"]`, d.Get("runID").Int()))
		if runNode == nil {
			return
		}
		runBox, ok := runNode.(browser.Element)
		if !ok || runBox.Attribute("data-run-id") != strconv.Itoa(d.Get("runID").Int()) {
			return
		}

		switch messageName {
		case "exit":
			exitStatusTemplate := /* language=gohtml */ ` <div class="exit-status"><pre>exit code {{.ExitCode}} after {{.Duration.String}}</pre></div>`
			exitCode := d.Get("exitCode").Int()

			duration := time.Duration(d.Get("duration").Int()) * time.Millisecond
			exitStatusTemplate = strings.Replace(exitStatusTemplate, `{{.ExitCode}}`, strconv.Itoa(exitCode), 1)
			exitStatusTemplate = strings.Replace(exitStatusTemplate, `{{.Duration.String}}`, duration.String(), 1)
			runBox.InsertAdjacentHTML(dom.PositionBeforeEnd, exitStatusTemplate)
		case "writeSync":
			writeSyncBuf := make([]byte, d.Get("buf").Length())
			js.CopyBytesToGo(writeSyncBuf, d.Get("buf"))
			writeSyncMessage := struct {
				Buf string
				FD  int
			}{
				Buf: string(writeSyncBuf),
				FD:  d.Get("fd").Int(),
			}
			stdout := runBox.QuerySelector(".stdout")
			stdout.Append(window.Document.CreateTextNode(writeSyncMessage.Buf))
		}
	})
	defer messageHandler.Release()
	window.AddEventListener("message", messageHandler)

	select {}
}

func handleRun(runID int) {
	codeTextareaEl := window.Document.QuerySelector("textarea#code").(browser.Element)
	codeTextarea := browser.Input(codeTextareaEl).Value()

	req, err := http.NewRequest(http.MethodPost, "/go/run", strings.NewReader(codeTextarea))
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
		window.Document.QuerySelector("pre#stderr").ReplaceChildren(window.Document.CreateTextNode(string(buf)))
		return
	}
	window.Document.QuerySelector("pre#stderr").ReplaceChildren()

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
			window.Document.QuerySelector("pre#stdout").ReplaceChildren(window.Document.CreateTextNode(string(buf)))
		case "stderr":
			buf, err := io.ReadAll(part)
			if err != nil {
				fmt.Println("failed to read response stdout", err)
				return
			}
			window.Document.QuerySelector("pre#stderr").ReplaceChildren(window.Document.CreateTextNode(string(buf)))
		case "output":
			buf, err := ioutil.ReadAll(part)
			if err != nil {
				fmt.Println("failed to read response output", err)
				return
			}
			runWASM(runID, buf)
		}
	}
}

func runWASM(runID int, buf []byte) {
	const runHTML = /* language=html */ `<div class="run" data-run-id="">
		<button class="close" style="font-weight: bolder; color: var(--fuchsia)">x</button>
		<input id="hide-window" type="checkbox" style="display: none;">
		<label for="hide-window" class="button" style="font-weight: bolder;"></label>
		<iframe
				class="run"
				srcdoc=""
				title="Run"
				sandbox="allow-scripts"
		></iframe>
		<pre class="stdout"></pre>
	</div>`

	origin := js.Global().Get("location").Get("origin").String()

	runIDString := strconv.Itoa(runID)

	htmlTemplate := window.Document.CreateElement("html").(browser.Element)
	htmlTemplate.InsertAdjacentHTML(dom.PositionAfterBegin, runHTMLHEAD)
	htmlTemplate.QuerySelector(`meta[name="go-playground-webapp-location"]`).SetAttribute("content", origin)
	htmlTemplate.QuerySelector(`meta[name="go-playground-run-id"]`).SetAttribute("content", runIDString)
	wasmExecScriptEl := window.Document.CreateElement("script")
	wasmExecScriptEl.ReplaceChildren(window.Document.CreateTextNode(wasmExec))
	htmlTemplate.InsertBefore(wasmExecScriptEl, htmlTemplate.QuerySelector("script"))
	htmlTemplate.InsertAdjacentHTML(dom.PositionBeforeEnd, "<body></body>")

	var sb strings.Builder
	sb.WriteString("<!doctype html>")
	sb.WriteString(htmlTemplate.OuterHTML())

	temporaryNode := window.Document.CreateElement("div").(browser.Element)
	temporaryNode.SetInnerHTML(runHTML)
	temporaryNode.QuerySelector("iframe.run").SetAttribute("srcdoc", sb.String())
	runBox := temporaryNode.FirstChild().(browser.Element)

	runBox.SetAttribute("data-run-id", runIDString)

	frame := runBox.QuerySelector("iframe.run").(browser.Element)

	var resources []interface {
		Release()
	}

	loadEventListener := browser.NewEventListenerFunc(func(event browser.Event) {
		message := window.NewObject()
		message.Set("name", "binary")
		array := window.NewUint8ClampedArray(len(buf))
		_ = js.CopyBytesToJS(array, buf)
		message.Set("binary", array)
		js.Value(frame).Get("contentWindow").Call("postMessage", message, "*")
	})
	resources = append(resources, loadEventListener)
	frame.AddEventListener("load", loadEventListener)

	closeBtn := runBox.QuerySelector("button.close").(browser.Element)

	closeEventListener := browser.NewEventListenerFunc(func(event browser.Event) {
		divRun := runBox.Closest("div.run")
		js.Value(frame).Get("contentWindow").Call("close")
		divRun.ParentElement().RemoveChild(divRun)

		for _, resource := range resources {
			resource.Release()
		}
	})
	resources = append(resources, closeEventListener)
	closeBtn.AddEventListener("click", closeEventListener)

	window.Document.QuerySelector("main").Append(runBox)
}

func updateExampleCodeHandler(event browser.Event) {
	updateExampleCode(event.Target().(dom.InputElement).Value())
}

func updateExampleCode(name string) {
	f, _ := examplesFS.Open(filepath.Join("examples", name, "main.go"))
	defer func() {
		_ = f.Close()
	}()
	buf, _ := io.ReadAll(f)
	browser.Input(window.Document.QuerySelector("textarea#code").(browser.Element)).SetValue(string(buf))
}
