package insights

import (
	"fmt"
	"strings"
)

func BuildPrompt(disciplines []DisciplinePerformance) string {
	var builder strings.Builder
	builder.WriteString("Voce e um mentor educacional para estudantes do ENEM. ")
	builder.WriteString("Gere um insight motivacional curto, pratico e acolhedor em portugues do Brasil. ")
	builder.WriteString("Nao crie cronograma e nao invente dados. Foque em orientar o aluno com base nas disciplinas de menor desempenho.\n\n")

	if len(disciplines) == 0 {
		builder.WriteString("Ainda nao ha dados suficientes de desempenho por disciplina. Gere uma mensagem encorajando consistencia e revisao dos primeiros resultados.")
		return builder.String()
	}

	builder.WriteString("Disciplinas de menor desempenho:\n")
	for _, discipline := range disciplines {
		builder.WriteString(fmt.Sprintf("- %s: %.2f%%\n", discipline.Nome, discipline.Percentual))
	}

	builder.WriteString("\nFormato desejado: um paragrafo de 3 a 5 frases, sem markdown.")
	return builder.String()
}
