package calendar

import (
	"embed"
	"net/http"
)

//go:embed templates/*.html
var TemplatesFS embed.FS

//go:embed docs/help.md
var helpMarkdown []byte

func handleHelpMarkdown(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown")
	w.Write(helpMarkdown)
}
