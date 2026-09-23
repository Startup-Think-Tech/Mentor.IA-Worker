package insights

import (
	"strings"
	"testing"
)

func TestBuildPromptWithDisciplines(t *testing.T) {
	prompt := BuildPrompt([]DisciplinePerformance{
		{Nome: "Matematica", Percentual: 42.5},
		{Nome: "Linguagens", Percentual: 55},
	})

	expectedParts := []string{
		"mentor educacional especializado no ENEM",
		"Use apenas os dados fornecidos",
		"um unico paragrafo",
		"acao pequena e concreta para hoje",
		"Matematica: 42.50%",
		"Linguagens: 55.00%",
	}

	for _, part := range expectedParts {
		if !strings.Contains(prompt, part) {
			t.Fatalf("prompt does not contain %q", part)
		}
	}
}

func TestBuildPromptWithoutDisciplines(t *testing.T) {
	prompt := BuildPrompt(nil)

	if !strings.Contains(prompt, "ainda nao ha desempenho suficiente por disciplina") {
		t.Fatal("prompt should explain missing performance data")
	}

	if !strings.Contains(prompt, "registro das primeiras sessoes") {
		t.Fatal("prompt should ask for initial study records")
	}
}
