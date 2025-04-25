package helper

import (
	"bytes"
	"text/template"

	"github.com/orange-juzipi/notify/pkg/github"
)

// RenderTemplate 渲染通知模板
func RenderTemplate(tmpl *template.Template, release *github.ReleaseInfo) (string, error) {
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, release); err != nil {
		return "", err
	}
	return buf.String(), nil
}
