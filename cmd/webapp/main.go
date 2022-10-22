//go:build js && wasm

package main

import (
	"embed"
	"fmt"
	"io"
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
	"github.com/crhntr/window/dom"

	"github.com/crhntr/playground"
)

var (
	//go:embed run_head.html
	runHead string

	//go:embed assets/wasm_exec.js
	wasmExec string

	//go:embed examples
	examplesFS embed.FS
)

func main() {
	exampleNames := playground.ListFileNames(examplesFS, "examples")
	selectExample := window.Document().QuerySelector("select#select-example")
	for _, name := range exampleNames {
		option := window.Document().CreateElement("option")
		option.SetAttribute("value", name)
		node := window.Document().CreateTextNode(name)
		option.AppendChild(node)
		selectExample.Append(option)
	}
	updateExampleCode(exampleNames[0])

	fn := window.WrapEventListenerFunc(updateExampleCodeHandler)
	defer fn.Release()
	selectExample.AddEventListener("change", fn, dom.AddEventListenerOptions{}, false)

	runButton := window.Document().QuerySelector("button#run")
	var runCount int64
	runBtnClickHandler := window.WrapEventListenerFunc(func(event dom.InputEvent) {
		atomic.AddInt64(&runCount, 1)

		codeTextareaEl := window.Document().QuerySelector("textarea#code")
		codeTextarea := dom.GetValue(codeTextareaEl)

		go handleBrowserRun(int(runCount), codeTextarea)
	})
	defer runBtnClickHandler.Release()
	runButton.AddEventListener("click", runBtnClickHandler, dom.AddEventListenerOptions{}, false)

	editorEl := window.Document().QuerySelector("#editor")
	editorEl.RemoveAttribute("hidden")
	defer editorEl.SetAttribute("hidden", "")

	messageHandler := window.WrapEventListenerFunc(handleMessageEvent)
	defer messageHandler.Release()
	window.Window().AddEventListener("message", messageHandler, dom.AddEventListenerOptions{}, false)

	select {}
}

func handleMessageEvent(event dom.MessageEvent) {
	d := js.Value(event).Get("data")
	messageName := d.Get("name").String()

	runBox := window.Document().QuerySelector(fmt.Sprintf(`#run-boxes [data-run-id="%d"]`, d.Get("runID").Int()))
	if runBox == nil {
		return
	}
	if runBox.GetAttribute("data-run-id") != strconv.Itoa(d.Get("runID").Int()) {
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
		stdout.Append(window.Document().CreateTextNode(writeSyncMessage.Buf))
	}
}

func handleBrowserRun(runID int, mainGo string) {
	req, err := http.NewRequest(http.MethodPost, "/go/run", strings.NewReader(mainGo))
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
		window.Document().QuerySelector("pre#stderr").ReplaceChildren(window.Document().CreateTextNode(string(buf)))
		return
	}
	window.Document().QuerySelector("pre#stderr").ReplaceChildren()

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
			window.Document().QuerySelector("pre#stdout").ReplaceChildren(window.Document().CreateTextNode(string(buf)))
		case "stderr":
			buf, err := io.ReadAll(part)
			if err != nil {
				fmt.Println("failed to read response stdout", err)
				return
			}
			window.Document().QuerySelector("pre#stderr").ReplaceChildren(window.Document().CreateTextNode(string(buf)))
		case "output":
			buf, err := io.ReadAll(part)
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

	htmlTemplate := window.Document().CreateElement("html")
	htmlTemplate.SetInnerHTML(runHead)
	htmlTemplate.QuerySelector(`meta[name="go-playground-webapp-location"]`).SetAttribute("content", origin)
	htmlTemplate.QuerySelector(`meta[name="go-playground-run-id"]`).SetAttribute("content", runIDString)
	htmlTemplate.QuerySelector(`script[id="run"]`).InsertAdjacentText(dom.PositionAfterBegin, wasmExec)
	htmlTemplate.InsertAdjacentHTML(dom.PositionBeforeEnd, "<body></body>")

	var sb strings.Builder
	sb.WriteString("<!doctype html>")
	sb.WriteString(htmlTemplate.OuterHTML())

	temporaryNode := window.Document().CreateElement("div")
	temporaryNode.SetInnerHTML(runHTML)
	temporaryNode.QuerySelector("iframe.run").SetAttribute("srcdoc", sb.String())
	runBox := temporaryNode.FirstElementChild()

	runBox.SetAttribute("data-run-id", runIDString)

	frame := runBox.QuerySelector("iframe.run")

	var resources []interface {
		Release()
	}

	loadEventListener := window.WrapEventListenerFunc(func(event dom.GenericEvent) {
		message := window.NewObject()
		message.Set("name", "binary")
		array := window.NewUint8ClampedArray(len(buf))
		_ = js.CopyBytesToJS(array, buf)
		message.Set("binary", array)
		dom.IFrameContentWindow(frame).PostMessage(message, "*")
	})
	resources = append(resources, loadEventListener)
	frame.AddEventListener("load", loadEventListener, dom.AddEventListenerOptions{}, false)

	closeBtn := runBox.QuerySelector("button.close")

	closeEventListener := window.WrapEventListenerFunc(func(event dom.GenericEvent) {
		divRun := runBox.Closest("div.run")
		dom.IFrameContentWindow(frame).Close()
		divRun.ParentElement().RemoveChild(divRun)
		for _, resource := range resources {
			resource.Release()
		}
	})
	resources = append(resources, closeEventListener)
	closeBtn.AddEventListener("click", closeEventListener, dom.AddEventListenerOptions{}, false)

	window.Document().QuerySelector("#run-boxes").Prepend(runBox)
}

func updateExampleCodeHandler(event dom.UIEvent) {
	updateExampleCode(dom.GetValue(dom.HTMLElement(event.Target())))
}

func updateExampleCode(name string) {
	f, _ := examplesFS.Open(filepath.Join("examples", name, "main.go"))
	defer func() {
		_ = f.Close()
	}()
	buf, _ := io.ReadAll(f)
	dom.SetValue(window.Document().QuerySelector("textarea#code"), string(buf))
}
