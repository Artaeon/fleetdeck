package server

import (
	"net/http"

	"github.com/fleetdeck/fleetdeck/internal/templates"
)

type apiTemplate struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (s *Server) handleListTemplates(w http.ResponseWriter, r *http.Request) {
	tmplList := templates.List()
	result := make([]apiTemplate, 0, len(tmplList))
	for _, t := range tmplList {
		result = append(result, apiTemplate{
			Name:        t.Name,
			Description: t.Description,
		})
	}
	writeJSON(w, result)
}
