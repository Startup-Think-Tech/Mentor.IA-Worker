-- name: FindLowestPerformanceDisciplines :many
SELECT d.id, d.nome, AVG(rd.percentual)::float8 AS percentual_medio
FROM registros_desempenho rd
JOIN disciplinas d ON d.id = rd.disciplina_id
WHERE rd.aluno_id = $1 AND d.ativo = true
GROUP BY d.id, d.nome
ORDER BY percentual_medio ASC, d.nome ASC
LIMIT $2;
