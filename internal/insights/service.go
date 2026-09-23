package insights

import (
	"context"
	"fmt"
)

type Processor interface {
	ProcessWithFakeResult(ctx context.Context, message Message) error
}

type Service struct {
	processor Processor
}

func NewService(processor Processor) *Service {
	return &Service{processor: processor}
}

func (s *Service) Process(ctx context.Context, message Message) error {
	if s.processor == nil {
		return fmt.Errorf("processador de insights nao configurado")
	}

	if err := s.processor.ProcessWithFakeResult(ctx, message); err != nil {
		return fmt.Errorf("falha ao processar insight: %w", err)
	}

	return nil
}
