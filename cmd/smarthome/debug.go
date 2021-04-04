package main

import (
	"html/template"
	"net/http"
)

func HandleDebug(w http.ResponseWriter, r *http.Request) {
	templateHtml := `<html><ul>{{range $key, $val := .}}
	    <li><strong>{{ $key }}</strong>: {{ $val }}</li>{{end}}</ul></html>`

	t := template.New("t")
	tmpl, err := t.Parse(templateHtml)
	if err != nil {
		errStr := "Debug template failed: " + err.Error()
		http.Error(w, errStr, http.StatusInternalServerError)
		return
	}
	tmpl.Execute(w, devices)
}
