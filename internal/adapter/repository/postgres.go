package repository

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/hack-fiap233/videos/internal/domain"
)

type PostgresRepository struct {
	db *sql.DB
}

func NewPostgresRepository(db *sql.DB) *PostgresRepository {
	return &PostgresRepository{db: db}
}

func (r *PostgresRepository) CreateTable(ctx context.Context) error {
	_, err := r.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS videos (
		id SERIAL PRIMARY KEY,
		title TEXT NOT NULL,
		description TEXT NOT NULL,
		status TEXT NOT NULL DEFAULT 'pending',
		s3_key TEXT
	)`)
	if err != nil {
		return fmt.Errorf("create table: %w", err)
	}

	migrations := []string{
		`ALTER TABLE videos ADD COLUMN IF NOT EXISTS status TEXT NOT NULL DEFAULT 'pending'`,
		`ALTER TABLE videos ADD COLUMN IF NOT EXISTS s3_key TEXT`,
	}
	for _, m := range migrations {
		if _, err := r.db.ExecContext(ctx, m); err != nil {
			return fmt.Errorf("migration: %w", err)
		}
	}
	return nil
}

func (r *PostgresRepository) Save(ctx context.Context, title, description string) (int, error) {
	var id int
	err := r.db.QueryRowContext(ctx,
		"INSERT INTO videos (title, description, status) VALUES ($1, $2, 'pending') RETURNING id",
		title, description,
	).Scan(&id)
	return id, err
}

func (r *PostgresRepository) UpdateS3Key(ctx context.Context, id int, s3Key string) error {
	_, err := r.db.ExecContext(ctx, "UPDATE videos SET s3_key = $1 WHERE id = $2", s3Key, id)
	return err
}

func (r *PostgresRepository) GetByID(ctx context.Context, id int) (*domain.Video, error) {
	var v domain.Video
	err := r.db.QueryRowContext(ctx,
		"SELECT id, title, description, status, COALESCE(s3_key, ''), COALESCE(zip_s3_key, '') FROM videos WHERE id = $1", id,
	).Scan(&v.ID, &v.Title, &v.Description, &v.Status, &v.S3Key, &v.ZipS3Key)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	return &v, err
}

func (r *PostgresRepository) List(ctx context.Context) ([]domain.Video, error) {
	rows, err := r.db.QueryContext(ctx,
		"SELECT id, title, description, status, COALESCE(s3_key, ''), COALESCE(zip_s3_key, '') FROM videos ORDER BY id",
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	videos := []domain.Video{}
	for rows.Next() {
		var v domain.Video
		if err := rows.Scan(&v.ID, &v.Title, &v.Description, &v.Status, &v.S3Key, &v.ZipS3Key); err != nil {
			continue
		}
		videos = append(videos, v)
	}
	return videos, nil
}

func (r *PostgresRepository) Ping(ctx context.Context) error {
	return r.db.PingContext(ctx)
}
