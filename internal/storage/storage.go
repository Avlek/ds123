package storage

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Storage struct {
	pool *pgxpool.Pool
}

func New(pool *pgxpool.Pool) *Storage {
	return &Storage{pool: pool}
}

func (s *Storage) GetMessages(ctx context.Context, sessionID string) ([]byte, error) {
	var data []byte
	err := s.pool.QueryRow(ctx, "SELECT messages FROM sessions WHERE session_id = $1", sessionID).Scan(&data)
	if errors.Is(err, pgx.ErrNoRows) {
		return []byte("[]"), nil
	}

	return data, err
}

func (s *Storage) SaveMessages(ctx context.Context, sessionID string, data []byte) error {
	_, err := s.pool.Exec(ctx, `INSERT INTO sessions (session_id, messages) 
		VALUES ($1, $2) ON CONFLICT (session_id) DO UPDATE SET messages = $2`, sessionID, data)

	return err
}
