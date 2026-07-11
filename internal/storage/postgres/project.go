package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/shikanon/orag/internal/project"
)

var _ project.Repository = (*ProjectRepository)(nil)

type ProjectRepository struct {
	db projectDB
}

type projectRow interface {
	Scan(...any) error
}

type projectRows interface {
	Next() bool
	Scan(...any) error
	Close()
	Err() error
}

type projectTx interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Commit(context.Context) error
	Rollback(context.Context) error
}

type projectDB interface {
	BeginProjectTx(context.Context) (projectTx, error)
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	Query(context.Context, string, ...any) (projectRows, error)
	QueryRow(context.Context, string, ...any) projectRow
}

func (r *ProjectRepository) CreateWithEnvironments(ctx context.Context, item project.Project, environments []project.Environment) error {
	tx, err := r.db.BeginProjectTx(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if _, err = tx.Exec(ctx, `
		INSERT INTO projects(id, tenant_id, name, description, created_at, updated_at)
		VALUES($1,$2,$3,$4,$5,$6)`, item.ID, item.TenantID, item.Name, item.Description, item.CreatedAt, item.UpdatedAt); err != nil {
		return projectPersistenceError(err)
	}
	for _, environment := range environments {
		if _, err = tx.Exec(ctx, `
			INSERT INTO project_environments(id, project_id, kind, created_at, updated_at)
			VALUES($1,$2,$3,$4,$5)`, environment.ID, environment.ProjectID, environment.Kind, environment.CreatedAt, environment.UpdatedAt); err != nil {
			return projectPersistenceError(err)
		}
	}
	return projectPersistenceError(tx.Commit(ctx))
}

func (r *ProjectRepository) List(ctx context.Context, tenantID string) ([]project.Project, error) {
	rows, err := r.db.Query(ctx, `
		SELECT id, tenant_id, name, description, created_at, updated_at
		FROM projects
		WHERE tenant_id=$1
		ORDER BY updated_at DESC`, tenantID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	items := make([]project.Project, 0)
	for rows.Next() {
		item, err := scanProject(rows)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return items, nil
}

func (r *ProjectRepository) Get(ctx context.Context, tenantID, id string) (project.Project, bool, error) {
	item, err := scanProject(r.db.QueryRow(ctx, `
		SELECT id, tenant_id, name, description, created_at, updated_at
		FROM projects
		WHERE tenant_id=$1 AND id=$2`, tenantID, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return project.Project{}, false, nil
	}
	if err != nil {
		return project.Project{}, false, err
	}
	return item, true, nil
}

func (r *ProjectRepository) Update(ctx context.Context, item project.Project) error {
	_, err := r.db.Exec(ctx, `
		UPDATE projects
		SET name=$3, description=$4, updated_at=$5
		WHERE tenant_id=$1 AND id=$2`, item.TenantID, item.ID, item.Name, item.Description, item.UpdatedAt)
	return projectPersistenceError(err)
}

func projectPersistenceError(err error) error {
	if err == nil {
		return nil
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		switch pgErr.Code {
		case "23503", "23505", "23514", "23P01":
			return errors.Join(project.ErrConflict, err)
		}
	}
	return err
}

func scanProject(row interface{ Scan(...any) error }) (project.Project, error) {
	var item project.Project
	err := row.Scan(&item.ID, &item.TenantID, &item.Name, &item.Description, &item.CreatedAt, &item.UpdatedAt)
	return item, err
}
