package ai

import "strings"

// ApplyTemplate substitutes {{key}} placeholders in a prompt template.
// Unknown placeholders are left untouched (harmless).
func ApplyTemplate(tmpl string, vars map[string]string) string {
	for k, v := range vars {
		tmpl = strings.ReplaceAll(tmpl, "{{"+k+"}}", v)
	}
	return tmpl
}
