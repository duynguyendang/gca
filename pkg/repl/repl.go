package repl

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	"github.com/duynguyendang/gca/pkg/meb"
	"github.com/google/generative-ai-go/genai"
	"google.golang.org/api/option"
)

// Run starts the interactive REPL.
func Run(s *meb.MEBStore, readOnly bool) {
	fmt.Println("\n--- Interactive Query Mode ---")

	// Recalculate stats to ensure we have fresh counts
	if !readOnly {
		if _, err := s.RecalculateStats(); err != nil {
			log.Printf("Stats recalc error: %v", err)
		}
	}
	fmt.Printf("Total Facts: %d\n", s.Count())
	predsList := s.ListPredicates()
	fmt.Printf("Total Predicates: %d\n", len(predsList))
	for _, p := range predsList {
		fmt.Printf(" - %s\n", p)
	}

	fmt.Println("Enter datalog queries (e.g. triples(S, \"calls\", O)). Type 'exit' or 'quit' to stop.")
	scanner := bufio.NewScanner(os.Stdin)

	// Load the prompt template at startup
	promptPath := "prompts/nl_to_datalog.prompt"
	nlPrompt, err := LoadPrompt(promptPath)
	if err != nil {
		log.Printf("Warning: Failed to load prompt from %s: %v. NL features may not work.", promptPath, err)
	}

	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "exit" || line == "quit" {
			break
		}
		if line == "" {
			continue
		}

		// Try to interpret as Datalog query first (if it looks like one?)
		// Actually, let's treat anything that doesn't start with "triples(" as a potential NL query
		// UNLESS it looks like a standard datalog query (e.g. predicates we know).
		// But relying on "triples" parser is safe.
		// If it has spaces and no '(', it's likely NL.
		isNL := !strings.Contains(line, "(") && strings.Contains(line, " ")

		if isNL {
			if nlPrompt == nil {
				fmt.Println("Error: NL prompt not loaded.")
				continue
			}
			fmt.Println("Thinking...")
			// Pass context facts (predicates) to help the LLM
			// Convert predicates to strings
			var factStrings []string
			for _, p := range predsList {
				factStrings = append(factStrings, fmt.Sprintf("%v", p))
			}
			nlQuery, err := askGemini(context.Background(), nlPrompt, line, factStrings)

			if err != nil {
				fmt.Printf("Gemini Error: %v\n", err)
				// Fallback to trying s.Query just in case
			} else {
				fmt.Printf("Translated to: %s\n", nlQuery)
				// recursing essentially
				line = nlQuery
			}
		}

		results, err := s.Query(context.Background(), line)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			continue
		}
		if len(results) == 0 {
			fmt.Println("[No results]")
			continue
		}
		for i, r := range results {
			if i >= 10 {
				fmt.Printf("... and %d more\n", len(results)-10)
				break
			}
			fmt.Printf("- %v\n", r)
		}
	}
	fmt.Println("Bye!")
}

func parseArg(s string) string {
	s = strings.TrimSpace(s)
	// If it starts with uppercase, it's a variable -> empty string for Scan
	if len(s) > 0 && s[0] >= 'A' && s[0] <= 'Z' {
		return ""
	}
	// If it's quoted, strip quotes
	return clean(s)
}

func clean(s string) string {
	return strings.TrimSpace(strings.ReplaceAll(s, "\"", ""))
}

func askGemini(ctx context.Context, p *Prompt, question string, facts []string) (string, error) {

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("GEMINI_API_KEY not set")
	}

	// Add timeout to prevent hanging
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second) // Increased timeout slightly
	defer cancel()

	client, err := genai.NewClient(ctx, option.WithAPIKey(apiKey))
	if err != nil {
		return "", err
	}
	defer client.Close()

	model := client.GenerativeModel(p.Config.Model)
	if model == nil {
		// Fallback if config model is empty ?? or just let it default?
		// GenerativeModel returns a value, so we just use what we have.
		// If p.Config.Model is empty, it might fail. Let's assume the prompt file is valid.
		if p.Config.Model == "" {
			model = client.GenerativeModel("gemini-3-flash-preview")
		}
	}
	model.SetTemperature(p.Config.Temperature)

	// Prepare data for template
	data := map[string]interface{}{
		"query":         question,
		"context_facts": facts,
	}

	promptStr, err := p.Execute(data)
	if err != nil {
		return "", fmt.Errorf("failed to execute prompt template: %w", err)
	}

	resp, err := model.GenerateContent(ctx, genai.Text(promptStr))

	if err != nil {
		return "", err
	}

	if len(resp.Candidates) == 0 || resp.Candidates[0].Content == nil {
		return "", fmt.Errorf("no response from Gemini")
	}

	for _, part := range resp.Candidates[0].Content.Parts {
		if txt, ok := part.(genai.Text); ok {
			clean := strings.TrimSpace(string(txt))
			// Remove markdown code blocks if Gemini adds them
			clean = strings.TrimPrefix(clean, "```datalog")
			clean = strings.TrimPrefix(clean, "```")
			clean = strings.TrimSuffix(clean, "```")
			return strings.TrimSpace(clean), nil
		}
	}
	return "", fmt.Errorf("unexpected response format")
}
