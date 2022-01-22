package view

import (
	_ "embed"

	"github.com/crhntr/window/dom"
)

type IndexData struct {
	GoVersion string
	Copyright string
}

func (data IndexData) Update(body dom.Element) {
	if data.GoVersion != "" {
		el := body.GetElementByID("go-version")
		el.SetTextContent("Using Go version " + data.GoVersion)
	}
	if data.Copyright != "" {
		el := body.GetElementByID("copyright-notice")
		el.SetTextContent(data.Copyright)
	}
}
