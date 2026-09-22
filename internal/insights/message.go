package insights

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Message struct {
	JobID   string `json:"job_id"`
	AlunoID string `json:"aluno_id"`
}

func ParseMessage(body []byte) (Message, error) {
	var message Message
	if err := json.Unmarshal(body, &message); err != nil {
		return Message{}, fmt.Errorf("mensagem de insight nao esta em JSON valido: %w", err)
	}

	message.JobID = strings.TrimSpace(message.JobID)
	message.AlunoID = strings.TrimSpace(message.AlunoID)

	if message.JobID == "" {
		return Message{}, fmt.Errorf("mensagem de insight sem job_id")
	}

	if message.AlunoID == "" {
		return Message{}, fmt.Errorf("mensagem de insight sem aluno_id")
	}

	return message, nil
}
