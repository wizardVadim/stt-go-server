package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"stt-service/internal/core/domains/transcription"
	"stt-service/internal/core/utils"
)

type DBTX interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type SQLiteRepository struct {
	db DBTX
}

func NewSQLiteRepository(db DBTX) *SQLiteRepository {
	return &SQLiteRepository{db: db}
}

func (repo *SQLiteRepository) Create(
	ctx context.Context,
	job transcription.Job,
) error {
	query := `
		INSERT INTO jobs 
		(id, original_filename, source_path, size_bytes, duration_seconds, status, progress, error_code, error_message, created_at, updated_at, finished_at, removed_at)
		VALUES
		($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
	`

	result, err := repo.db.ExecContext(
		ctx,
		query,
		job.ID(),
		job.OriginalName(),
		job.SourcePath(),
		job.SizeBytes(),
		job.Duration().Seconds(),
		job.Status(),
		job.Progress(),
		job.ErrorCode(),
		job.ErrorMessage(),
		utils.FormatStorageTime(job.CreatedAt()),
		utils.FormatStorageTime(job.UpdatedAt()),
		utils.FormatOptionalStorageTime(job.FinishedAt()),
		utils.FormatOptionalStorageTime(job.RemovedAt()),
	)
	if err != nil {
		return fmt.Errorf("create job %s: %w", job.ID(), err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("%w: %s", err, "rows affected err")
	}
	if rows == 0 {
		return fmt.Errorf("%w: %+v", errInsertIntoTable, job.ID())
	}

	return nil
}

func (repo *SQLiteRepository) GetByID(
	ctx context.Context,
	id string,
) (transcription.Job, error) {
	query := `
		SELECT 
			id,
			original_filename,
			source_path,
			size_bytes,
			duration_seconds,
			status,
			progress,
			error_code,
			error_message,
			created_at,
			updated_at,
			finished_at,
			removed_at 
		FROM jobs WHERE id = $1
	`

	dao := JobDAO{}

	if err := repo.db.QueryRowContext(ctx, query, id).Scan(
		&dao.ID,
		&dao.OriginalFilename,
		&dao.SourcePath,
		&dao.SizeBytes,
		&dao.DurationSeconds,
		&dao.Status,
		&dao.Progress,
		&dao.ErrorCode,
		&dao.ErrorMessage,
		&dao.CreatedAt,
		&dao.UpdatedAt,
		&dao.FinishedAt,
		&dao.RemovedAt,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return transcription.Job{}, fmt.Errorf("%w: getting job by id '%s'", transcription.ErrJobNotFound, id)
		}
		return transcription.Job{}, fmt.Errorf("%w: getting job by id '%s'", err, id)
	}

	job, err := dao.ParseIntoDomain()
	if err != nil {
		return transcription.Job{}, fmt.Errorf("%w: parsing into domain; job id '%s'", err, id)
	}

	return job, nil
}

func (repo *SQLiteRepository) List(
	ctx context.Context,
) ([]transcription.Job, error) {
	query := `
		SELECT 
			id,
			original_filename,
			source_path,
			size_bytes,
			duration_seconds,
			status,
			progress,
			error_code,
			error_message,
			created_at,
			updated_at,
			finished_at,
			removed_at 
		FROM jobs
		WHERE removed_at IS NULL
		ORDER BY created_at DESC
	`

	rows, err := repo.db.QueryContext(ctx, query)
	if err != nil {
		return []transcription.Job{}, fmt.Errorf("%w: getting job list", err)
	}
	defer rows.Close()

	jobs := make([]transcription.Job, 0)

	for rows.Next() {

		dao := JobDAO{}

		if err := rows.Scan(
			&dao.ID,
			&dao.OriginalFilename,
			&dao.SourcePath,
			&dao.SizeBytes,
			&dao.DurationSeconds,
			&dao.Status,
			&dao.Progress,
			&dao.ErrorCode,
			&dao.ErrorMessage,
			&dao.CreatedAt,
			&dao.UpdatedAt,
			&dao.FinishedAt,
			&dao.RemovedAt,
		); err != nil {
			return []transcription.Job{}, fmt.Errorf("%w: getting job list", err)
		}

		job, err := dao.ParseIntoDomain()
		if err != nil {
			return []transcription.Job{}, fmt.Errorf("%w: parsing into domain; job id '%s'", err, dao.ID)
		}
		jobs = append(jobs, job)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate jobs: %w", err)
	}

	return jobs, nil
}

func (repo *SQLiteRepository) Update(
	ctx context.Context,
	job transcription.Job,
) error {
	query := `
		UPDATE jobs SET
			status = $1,
			progress = $2,
			error_code = $3,
			error_message = $4,
			updated_at = $5,
			finished_at = $6,
			removed_at = $7
		WHERE id = $8
	`

	result, err := repo.db.ExecContext(
		ctx,
		query,
		job.Status(),
		job.Progress(),
		job.ErrorCode(),
		job.ErrorMessage(),
		utils.FormatStorageTime(job.UpdatedAt()),
		utils.FormatOptionalStorageTime(job.FinishedAt()),
		utils.FormatOptionalStorageTime(job.RemovedAt()),
		job.ID(),
	)
	if err != nil {
		return fmt.Errorf("update job %s: %w", job.ID(), err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("update job %s rows affected: %w", job.ID(), err)
	}
	if rows == 0 {
		return fmt.Errorf("update job %s: %w", job.ID(), transcription.ErrJobNotFound)
	}

	return nil
}
