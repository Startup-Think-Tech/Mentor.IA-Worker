package postgres

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Client struct {
	pool *pgxpool.Pool
}

func Connect(ctx context.Context, databaseURL string) (*Client, error) {
	if databaseURL == "" {
		return nil, fmt.Errorf("DATABASE_URL nao pode estar vazio")
	}

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("falha ao criar pool do PostgreSQL: %w", err)
	}

	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("falha ao conectar no PostgreSQL: %w", err)
	}

	return &Client{pool: pool}, nil
}

func (c *Client) Pool() *pgxpool.Pool {
	return c.pool
}

func (c *Client) Close() {
	if c == nil || c.pool == nil {
		return
	}

	c.pool.Close()
}
