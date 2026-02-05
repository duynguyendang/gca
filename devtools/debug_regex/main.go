package main

import (
	"fmt"
	"regexp"
	"strings"
)

func main() {
	content := `
	s.router.GET("/v1/projects", s.handleProjects)
	s.router.POST("/v1/ai/ask", s.handleAIAsk)
	`

	routeRegex := regexp.MustCompile(`\.(GET|POST|PUT|DELETE|PATCH|OPTIONS|HEAD)\(\s*"([^"]+)"\s*,\s*([^,\)]+)`)

	matches := routeRegex.FindAllStringSubmatch(content, -1)
	fmt.Printf("Matches found: %d\n", len(matches))
	for _, match := range matches {
		fmt.Printf("Match: %q, Route: %q, Handler: %q\n", match[0], match[2], match[3])
		rawHandler := strings.TrimSpace(match[3])
		handlerToken := rawHandler
		if idx := strings.LastIndex(rawHandler, "."); idx != -1 {
			handlerToken = rawHandler[idx+1:]
		}
		handlerToken = strings.Trim(handlerToken, " ),;")
		fmt.Printf("  Token: %q\n", handlerToken)
	}
}
