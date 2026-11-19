package space

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jasonwa/goddi/internal/ipam/lifecycle"
)

// ErrSpaceInUse is returned when retiring a space that still has active subnets.
// Handlers map it to HTTP 409 instead of a misleading 500.
var ErrSpaceInUse = errors.New("space in use")

// ErrSpaceRetired means an active-only operation was aimed at a retired space.
var ErrSpaceRetired = errors.New("space is retired")

// ErrSpaceNotRetired means a restore was aimed at an active space.
var ErrSpaceNotRetired = errors.New("space is not retired")

// ErrSpaceNameInUse means the restored name would collide with an active space.
// Retirement frees the original name; restore cannot silently take it back.
var ErrSpaceNameInUse = errors.New("space name is in use")

// ErrSpaceHasChildren means a purge was aimed at a tombstone still holding subnets.
var ErrSpaceHasChildren = errors.New("space still has child subnets")

// ErrPurgeConfirmation means the caller did not name the exact resource being purged.
var ErrPurgeConfirmation = errors.New("purge confirmation must name the resource id")

// Space represents an IPAM address space.
type Space struct {
	ID             string `json:"id"`
	Name           string `json:"name"`
	Description    string `json:"description,omitempty"`
	LifecycleState string `json:"lifecycle_state"`
	RetiredAt      string `json:"retired_at,omitempty"`
	RetiredBy      string `json:"retired_by,omitempty"`
	RetireReason   string `json:"retire_reason,omitempty"`
	CreatedAt      string `json:"created_at"`
	UpdatedAt      string `json:"updated_at"`
}

// SpaceOptions holds parameters for creating or updating a space.
type SpaceOptions struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// SpaceFilter holds filter parameters for listing spaces.
type SpaceFilter struct {
	Name           string `json:"name,omitempty"`
	LifecycleState string `json:"lifecycle_state,omitempty"`
	Page           int    `json:"page,omitempty"`
	PageSize       int    `json:"page_size,omitempty"`
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
		INSERT INTO ipam_spaces (id, name, description, lifecycle_state, created_at, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		id, name, description, lifecycle.Active)
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
		SELECT id, name, description, lifecycle_state, COALESCE(retired_at, ''),
			COALESCE(retired_by, ''), COALESCE(retire_reason, ''), created_at, updated_at
		FROM ipam_spaces WHERE id = ?`, id,
	).Scan(&s.ID, &s.Name, &description, &s.LifecycleState, &s.RetiredAt, &s.RetiredBy, &s.RetireReason, &s.CreatedAt, &s.UpdatedAt)
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
	state := filter.LifecycleState
	if state == "" {
		state = lifecycle.Active
	}
	if state != "all" {
		conditions = append(conditions, "lifecycle_state = ?")
		args = append(args, state)
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

	querySQL := `SELECT id, name, description, lifecycle_state, COALESCE(retired_at, ''),
			COALESCE(retired_by, ''), COALESCE(retire_reason, ''), created_at, updated_at
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
		if err := rows.Scan(&s.ID, &s.Name, &description, &s.LifecycleState, &s.RetiredAt, &s.RetiredBy, &s.RetireReason, &s.CreatedAt, &s.UpdatedAt); err != nil {
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
	if existing.LifecycleState == lifecycle.Retired {
		return nil, ErrSpaceRetired
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

// RetireSpace marks a space as retired instead of deleting it.
//
// Active subnets still block retirement: a live DHCP scope must never point at
// a subnet whose parent has been taken out of service. Retired subnets remain
// attached to the space as archived children and do not block it. The space
// row itself stays so local-only ipam_addresses/ipam_history rows keep a valid
// FK parent and the retire state can reach followers through snapshots.
func (m *Manager) RetireSpace(id, actor, reason string) error {
	var subnetCount int64
	if err := m.db.QueryRow("SELECT COUNT(*) FROM ipam_subnets WHERE space_id = ? AND lifecycle_state = ?", id, lifecycle.Active).Scan(&subnetCount); err != nil {
		return fmt.Errorf("failed to check for active subnets in space %s: %w", id, err)
	}
	if subnetCount > 0 {
		return fmt.Errorf("%w: cannot retire space with active subnets (%d active subnets exist)", ErrSpaceInUse, subnetCount)
	}

	result, err := m.db.Exec(`UPDATE ipam_spaces SET
		lifecycle_state = ?, retired_at = datetime('now'), retired_by = ?, retire_reason = ?,
		name = name || '-retired-' || id, updated_at = datetime('now')
		WHERE id = ? AND lifecycle_state = ?`, lifecycle.Retired, actor, reason, id, lifecycle.Active)
	if err != nil {
		return fmt.Errorf("failed to retire space: %w", err)
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		return fmt.Errorf("space not found")
	}
	return nil
}

// RestoreSpace brings a retired tombstone back into active service.
//
// Retirement renames the parent to free its original name, so restore has to
// choose a name: the caller may supply one, or the original is recovered from
// the tombstone suffix. A name already used by an active space is a conflict,
// never an implicit overwrite. Retire metadata is cleared on success so the
// archive stops listing a resource that is live again.
func (m *Manager) RestoreSpace(id, name string) (*Space, error) {
	existing, err := m.GetSpace(id)
	if err != nil {
		return nil, err
	}
	if existing.LifecycleState != lifecycle.Retired {
		return nil, ErrSpaceNotRetired
	}
	if strings.TrimSpace(name) == "" {
		name = strings.TrimSuffix(existing.Name, "-retired-"+id)
	}
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("restored name is required")
	}

	tx, err := m.db.Begin()
	if err != nil {
		return nil, fmt.Errorf("begin restore: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var inUse int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM ipam_spaces WHERE name = ? AND id <> ? AND lifecycle_state = ?`, name, id, lifecycle.Active).Scan(&inUse); err != nil {
		return nil, fmt.Errorf("check restored name: %w", err)
	}
	if inUse > 0 {
		return nil, fmt.Errorf("%w: %s", ErrSpaceNameInUse, name)
	}
	if _, err := tx.Exec(`UPDATE ipam_spaces SET
			name = ?, lifecycle_state = ?, retired_at = NULL, retired_by = '', retire_reason = '',
			updated_at = datetime('now')
		WHERE id = ? AND lifecycle_state = ?`, name, lifecycle.Active, id, lifecycle.Retired); err != nil {
		return nil, fmt.Errorf("restore space: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit restore: %w", err)
	}
	return m.GetSpace(id)
}

// PurgeSpace permanently removes a retired space tombstone.
//
// This is deliberately narrower than "delete": only a retired tombstone with
// no child subnets can be purged. Anything else would either remove an active
// configuration or break the FK parent of preserved local audit rows. The
// caller must send the exact resource id as confirmation, so a stale dialog
// cannot purge a different tombstone.
func (m *Manager) PurgeSpace(id, confirmation string) error {
	s, err := m.GetSpace(id)
	if err != nil {
		return err
	}
	if s.LifecycleState != lifecycle.Retired {
		return ErrSpaceNotRetired
	}
	if confirmation != id {
		return ErrPurgeConfirmation
	}

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("begin purge: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	var children int64
	if err := tx.QueryRow(`SELECT COUNT(*) FROM ipam_subnets WHERE space_id = ?`, id).Scan(&children); err != nil {
		return fmt.Errorf("check purge children: %w", err)
	}
	if children > 0 {
		return fmt.Errorf("%w: %d subnets remain", ErrSpaceHasChildren, children)
	}
	if _, err := tx.Exec(`DELETE FROM ipam_spaces WHERE id = ? AND lifecycle_state = ?`, id, lifecycle.Retired); err != nil {
		return fmt.Errorf("purge space: %w", err)
	}
	return tx.Commit()
}

// DeleteSpace is kept as a compatibility alias; HTTP DELETE now retires.
func (m *Manager) DeleteSpace(id string) error { return m.RetireSpace(id, "", "") }
