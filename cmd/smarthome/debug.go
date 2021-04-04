package main

import (
	"html/template"
	"io/ioutil"
	"net/http"
)

func HandleDebug(w http.ResponseWriter, r *http.Request) {
	filename := "/tmp/debug.html"
	templateHtml := []byte("<html><ul>{{range .}} <li>{{.}}</li> {{end}}</ul></html>")
	err := ioutil.WriteFile(filename, templateHtml, 0666)
	if err != nil {
		w.Write([]byte("write template failed"))
		return
	}
	tmpl := template.Must(template.ParseFiles(filename))
	tmpl.Execute(w, devices)
}
