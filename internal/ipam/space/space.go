package space

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// ErrSpaceInUse is returned when deleting a space that still has subnets.
// Handlers map it to HTTP 409 instead of a misleading 500.
var ErrSpaceInUse = errors.New("space in use")

// Space represents an IPAM address space.
type Space struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	CreatedAt   string `json:"created_at"`
	UpdatedAt   string `json:"updated_at"`
}

// SpaceOptions holds parameters for creating or updating a space.
type SpaceOptions struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SpaceFilter holds filter parameters for listing spaces.
type SpaceFilter struct {
	Name     string `json:"name,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

// Manager provides CRUD operations for IPAM spaces.
type Manager struct {
	db *sql.DB
}

// NewManager creates a new space manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// CreateSpace creates a new IPAM space.
func (m *Manager) CreateSpace(name, description string) (*Space, error) {
	if name == "" {
		return nil, fmt.Errorf("name is required")
	}

	id := uuid.New().String()
	_, err := m.db.Exec(`
		INSERT INTO ipam_spaces (id, name, description, created_at, updated_at)
		VALUES (?, ?, ?, datetime('now'), datetime('now'))`,
		id, name, description)
	if err != nil {
		return nil, fmt.Errorf("failed to create space: %w", err)
	}

	return m.GetSpace(id)
}

// GetSpace retrieves a space by ID.
func (m *Manager) GetSpace(id string) (*Space, error) {
	s := &Space{}
	var description sql.NullString

	err := m.db.QueryRow(`
		SELECT id, name, description, created_at, updated_at
		FROM ipam_spaces WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &description, &s.CreatedAt, &s.UpdatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("space not found")
	}
	if err != nil {
		return nil, err
	}

	s.Description = description.String
	return s, nil
}

// ListSpaces lists spaces with filtering and pagination.
func (m *Manager) ListSpaces(filter SpaceFilter) ([]Space, int64, error) {
	var conditions []string
	var args []interface{}

	if filter.Name != "" {
		conditions = append(conditions, "name LIKE ?")
		args = append(args, "%"+filter.Name+"%")
	}

	whereClause := ""
	if len(conditions) > 0 {
		whereClause = "WHERE " + strings.Join(conditions, " AND ")
	}

	var total int64
	countSQL := "SELECT COUNT(*) FROM ipam_spaces " + whereClause
	if err := m.db.QueryRow(countSQL, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	page := filter.Page
	pageSize := filter.PageSize
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize

	querySQL := `SELECT id, name, description, created_at, updated_at
		FROM ipam_spaces ` + whereClause + ` ORDER BY created_at DESC LIMIT ? OFFSET ?`
	args = append(args, pageSize, offset)

	rows, err := m.db.Query(querySQL, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	var spaces []Space
	for rows.Next() {
		var s Space
		var description sql.NullString
		if err := rows.Scan(&s.ID, &s.Name, &description, &s.CreatedAt, &s.UpdatedAt); err != nil {
			return nil, 0, err
		}
		s.Description = description.String
		spaces = append(spaces, s)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	return spaces, total, nil
}

// UpdateSpace updates a space.
func (m *Manager) UpdateSpace(id string, opts SpaceOptions) (*Space, error) {
	existing, err := m.GetSpace(id)
	if err != nil {
		return nil, err
	}

	if opts.Name != "" {
		existing.Name = opts.Name
	}
	if opts.Description != "" {
		existing.Description = opts.Description
	}

	_, err = m.db.Exec(`
		UPDATE ipam_spaces SET name=?, description=?, updated_at=datetime('now')
		WHERE id=?`, existing.Name, existing.Description, id)
	if err != nil {
		return nil, fmt.Errorf("failed to update space: %w", err)
	}

	return m.GetSpace(id)
}

// DeleteSpace deletes a space. Checks for subnets first.
func (m *Manager) DeleteSpace(id string) error {
	var subnetCount int64
	m.db.QueryRow("SELECT COUNT(*) FROM ipam_subnets WHERE space_id = ?", id).Scan(&subnetCount)
	if subnetCount > 0 {
		return fmt.Errorf("%w: cannot delete space with subnets (%d subnets exist)", ErrSpaceInUse, subnetCount)
	}

	result, err := m.db.Exec("DELETE FROM ipam_spaces WHERE id = ?", id)
	if err != nil {
		return fmt.Errorf("failed to delete space: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("space not found")
	}
	return nil
}
