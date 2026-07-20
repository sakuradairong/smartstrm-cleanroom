package tmdb

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var tokenPattern = regexp.MustCompile(`\{([^{}]+)\}`)

func Render(template string, values map[string]string) (string, error) {
	var renderErr error
	result := tokenPattern.ReplaceAllStringFunc(template, func(token string) string {
		body := strings.TrimSuffix(strings.TrimPrefix(token, "{"), "}")
		prefix, variable, suffix := "", body, ""
		if comma := strings.Index(body, ","); comma >= 0 {
			prefix, variable = body[:comma], body[comma+1:]
			if second := strings.Index(variable, ","); second >= 0 {
				variable, suffix = variable[:second], variable[second+1:]
			}
		}
		width := 0
		if colon := strings.LastIndex(variable, ":"); colon >= 0 {
			parsed, err := strconv.Atoi(variable[colon+1:])
			if err != nil || parsed < 1 || parsed > 12 {
				renderErr = fmt.Errorf("invalid template width in %s", token)
				return ""
			}
			width, variable = parsed, variable[:colon]
		}
		value := values[variable]
		if value == "" {
			return ""
		}
		if width > 0 {
			if number, err := strconv.Atoi(value); err == nil {
				value = fmt.Sprintf("%0*d", width, number)
			}
		}
		return prefix + value + suffix
	})
	if renderErr != nil {
		wrapped := fmt.Errorf("render TMDB template: %w", renderErr)
		return "", wrapped
	}
	if strings.ContainsAny(result, `/\`) {
		err := fmt.Errorf("rendered name contains path separators")
		return "", err
	}
	trimmed := strings.TrimSpace(result)
	return trimmed, nil
}
