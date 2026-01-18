package main

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"html/template"
	"log"
	"reflect"
	"slices"
	"syscall/js"
)

//go:embed body.gohtml
var templateSource embed.FS

var templates = template.Must(template.ParseFS(templateSource, "*"))

func main() {
	var buf bytes.Buffer

	types := []reflect.Type{
		reflect.TypeFor[Basket](),
		reflect.TypeFor[Lock](),
	}

	if err := templates.ExecuteTemplate(&buf, "body", struct {
		Types []reflect.Type
	}{
		Types: types,
	}); err != nil {
		log.Fatalln(err)
	}
	js.Global().Get("document").Call("querySelector", "body").Call("insertAdjacentHTML", "beforeend", buf.String())

	textareaEl := js.Global().Get("document").Call("querySelector", "#input")
	errorEl := js.Global().Get("document").Call("querySelector", "#error")
	iframeEl := js.Global().Get("document").Call("querySelector", "#output")
	typesEl := js.Global().Get("document").Call("querySelector", "#types")
	dataEl := js.Global().Get("document").Call("querySelector", "#data")

	changeHandler := js.FuncOf(func(this js.Value, args []js.Value) any {
		executeTemplate(types, textareaEl, errorEl, iframeEl, dataEl, typesEl)
		return nil
	})
	textareaEl.Call("addEventListener", "change", changeHandler, js.ValueOf(false))
	typesEl.Call("addEventListener", "change", changeHandler, js.ValueOf(false))
	dataEl.Call("addEventListener", "change", changeHandler, js.ValueOf(false))

	executeTemplate(types, textareaEl, errorEl, iframeEl, dataEl, typesEl)

	select {}
}

func executeTemplate(types []reflect.Type, textareaElement, errorElement, outputElement, dataEl, typesEl js.Value) {
	if textareaElement.IsNull() {
		panic("textareaElement must not be nil")
	}
	if errorElement.IsNull() {
		panic("errorElement must not be nil")
	}
	if outputElement.IsNull() {
		panic("outputElement must not be nil")
	}
	if dataEl.IsNull() {
		panic("dataEl must not be nil")
	}
	if typesEl.IsNull() {
		panic("typesEl must not be nil")
	}

	typeValue := typesEl.Get("value").String()
	ti := slices.IndexFunc(types, func(r reflect.Type) bool {
		return typeValue == r.Name()
	})
	if ti < 0 {
		errorElement.Set("innerText", fmt.Sprintf("unknown type %s", typeValue))
		outputElement.Set("srcdoc", "")
		return
	}
	dataType := types[ti]

	dataJSON := dataEl.Get("value").String()
	if !json.Valid([]byte(dataJSON)) {
		errorElement.Set("innerText", "invalid data json")
		outputElement.Set("srcdoc", "")
		return
	}

	data := reflect.New(dataType).Interface()

	if err := recoverUnmarshal([]byte(dataJSON), data); err != nil {
		errorElement.Set("innerText", err.Error())
		outputElement.Set("srcdoc", "")
		return
	}

	textareaValue := textareaElement.Get("value").String()
	t, err := template.New("textarea").Parse(textareaValue)
	if err != nil {
		errorElement.Set("innerText", err.Error())
		outputElement.Set("srcdoc", "")
		return
	}

	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		errorElement.Set("innerText", err.Error())
		outputElement.Set("srcdoc", "")
		return
	}

	errorElement.Set("innerText", "")
	outputElement.Set("srcdoc", buf.String())
}

func recoverUnmarshal(dataJSON []byte, data any) (err error) {
	defer func() {
		// for some reason json.Unmarshal will panic when the type in the json can not be unmarshalled to data
		// AKA whey the types of json don't align with the data type
		if r := recover(); r != nil {
			err = fmt.Errorf("%s", r)
			return
		}
	}()
	return json.Unmarshal(dataJSON, data)
}
