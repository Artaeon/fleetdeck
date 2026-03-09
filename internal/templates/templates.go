package templates

import (
	"bytes"
	"fmt"
	"text/template"
)

type TemplateData struct {
	Name            string
	Domain          string
	PostgresVersion string
}

type Template struct {
	Name        string
	Description string
	Dockerfile  string
	Compose     string
	Workflow    string
	EnvTemplate string
	GitIgnore   string
}

var registry = map[string]*Template{}

func Register(t *Template) {
	registry[t.Name] = t
}

func Get(name string) (*Template, error) {
	t, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("template %q not found", name)
	}
	return t, nil
}

func List() []*Template {
	var templates []*Template
	for _, t := range registry {
		templates = append(templates, t)
	}
	return templates
}

func Render(tmpl string, data TemplateData) (string, error) {
	t, err := template.New("").Parse(tmpl)
	if err != nil {
		return "", err
	}
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}
