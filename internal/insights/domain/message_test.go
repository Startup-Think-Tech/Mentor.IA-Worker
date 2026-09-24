package domain

import "testing"

func TestParseMessage(t *testing.T) {
	message, err := ParseMessage([]byte(`{"job_id":"job-1","aluno_id":"aluno-1"}`))
	if err != nil {
		t.Fatalf("ParseMessage() returned error: %v", err)
	}

	if message.JobID != "job-1" {
		t.Fatalf("JobID = %q, want %q", message.JobID, "job-1")
	}

	if message.AlunoID != "aluno-1" {
		t.Fatalf("AlunoID = %q, want %q", message.AlunoID, "aluno-1")
	}
}

func TestParseMessageRejectsInvalidJSON(t *testing.T) {
	_, err := ParseMessage([]byte(`invalid`))
	if err == nil {
		t.Fatal("ParseMessage() returned nil error")
	}
}

func TestParseMessageRejectsMissingJobID(t *testing.T) {
	_, err := ParseMessage([]byte(`{"aluno_id":"aluno-1"}`))
	if err == nil {
		t.Fatal("ParseMessage() returned nil error")
	}
}

func TestParseMessageRejectsMissingAlunoID(t *testing.T) {
	_, err := ParseMessage([]byte(`{"job_id":"job-1"}`))
	if err == nil {
		t.Fatal("ParseMessage() returned nil error")
	}
}
