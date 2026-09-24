CREATE TABLE disciplinas (
    id UUID PRIMARY KEY,
    nome TEXT NOT NULL,
    ativo BOOLEAN NOT NULL
);

CREATE TABLE registros_desempenho (
    aluno_id UUID NOT NULL,
    disciplina_id UUID NOT NULL,
    percentual NUMERIC NOT NULL
);
