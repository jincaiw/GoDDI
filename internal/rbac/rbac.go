package rbac

import (
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Permission represents a permission in the system.
type Permission struct {
	ID          string `json:"id"`
	Resource    string `json:"resource"`
	Action      string `json:"action"`
	Description string `json:"description,omitempty"`
}

// Role represents a role in the system.
type Role struct {
	ID          string       `json:"id"`
	Name        string       `json:"name"`
	Description string       `json:"description,omitempty"`
	IsBuiltin   bool         `json:"is_builtin"`
	Permissions []Permission `json:"permissions,omitempty"`
	CreatedAt   time.Time    `json:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at"`
}

// Predefined permissions.
var PredefinedPermissions = []Permission{
	// DNS permissions
	{Resource: "dns", Action: "read", Description: "Read DNS configuration and records"},
	{Resource: "dns", Action: "write", Description: "Create and modify DNS records"},
	{Resource: "dns", Action: "delete", Description: "Delete DNS records and zones"},
	// DHCP permissions
	{Resource: "dhcp", Action: "read", Description: "Read DHCP configuration and leases"},
	{Resource: "dhcp", Action: "write", Description: "Create and modify DHCP scopes and reservations"},
	{Resource: "dhcp", Action: "delete", Description: "Delete DHCP scopes and reservations"},
	// IPAM permissions
	{Resource: "ipam", Action: "read", Description: "Read IPAM spaces, subnets, and addresses"},
	{Resource: "ipam", Action: "write", Description: "Create and modify IPAM resources"},
	{Resource: "ipam", Action: "delete", Description: "Delete IPAM resources"},
	// User management permissions
	{Resource: "user", Action: "read", Description: "Read user information"},
	{Resource: "user", Action: "write", Description: "Create and modify users"},
	{Resource: "user", Action: "delete", Description: "Delete users"},
	// Role management permissions
	{Resource: "role", Action: "read", Description: "Read role information"},
	{Resource: "role", Action: "write", Description: "Create and modify roles"},
	{Resource: "role", Action: "delete", Description: "Delete roles"},
	// Group management permissions
	{Resource: "group", Action: "read", Description: "Read group information"},
	{Resource: "group", Action: "write", Description: "Create and modify groups"},
	{Resource: "group", Action: "delete", Description: "Delete groups"},
	// Settings permissions
	{Resource: "settings", Action: "read", Description: "Read system settings"},
	{Resource: "settings", Action: "write", Description: "Modify system settings"},
	// Audit permissions
	{Resource: "audit", Action: "read", Description: "Read audit logs"},
	// Backup permissions
	{Resource: "backup", Action: "read", Description: "List and download backups"},
	{Resource: "backup", Action: "write", Description: "Create and restore backups"},
	// Token permissions
	{Resource: "token", Action: "read", Description: "List API tokens"},
	{Resource: "token", Action: "write", Description: "Create API tokens"},
	{Resource: "token", Action: "delete", Description: "Revoke API tokens"},
}

// Predefined role definitions.
var PredefinedRoles = []struct {
	Name        string
	Description string
	Permissions []Permission
}{
	{
		Name:        "admin",
		Description: "Administrator with full access to all resources",
		Permissions: PredefinedPermissions, // All permissions
	},
	{
		Name:        "operator",
		Description: "Operator with manage access to DNS, DHCP, and IPAM",
		Permissions: []Permission{
			{Resource: "dns", Action: "read"},
			{Resource: "dns", Action: "write"},
			{Resource: "dns", Action: "delete"},
			{Resource: "dhcp", Action: "read"},
			{Resource: "dhcp", Action: "write"},
			{Resource: "dhcp", Action: "delete"},
			{Resource: "ipam", Action: "read"},
			{Resource: "ipam", Action: "write"},
			{Resource: "ipam", Action: "delete"},
			{Resource: "settings", Action: "read"},
			{Resource: "audit", Action: "read"},
			{Resource: "backup", Action: "read"},
			{Resource: "group", Action: "read"},
		},
	},
	{
		Name:        "viewer",
		Description: "Read-only access to all resources",
		Permissions: []Permission{
			{Resource: "dns", Action: "read"},
			{Resource: "dhcp", Action: "read"},
			{Resource: "ipam", Action: "read"},
			{Resource: "user", Action: "read"},
			{Resource: "role", Action: "read"},
			{Resource: "settings", Action: "read"},
			{Resource: "audit", Action: "read"},
			{Resource: "backup", Action: "read"},
			{Resource: "token", Action: "read"},
			{Resource: "group", Action: "read"},
		},
	},
}

// RBACManager manages role-based access control.
type RBACManager struct {
	db *sql.DB
}

// NewRBACManager creates a new RBACManager.
func NewRBACManager(db *sql.DB) *RBACManager {
	return &RBACManager{db: db}
}

// InitializePredefinedData seeds the database with predefined roles and permissions.
func (rm *RBACManager) InitializePredefinedData() error {
	tx, err := rm.db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	// Insert predefined permissions
	for _, perm := range PredefinedPermissions {
		id := uuid.New().String()
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO permissions (id, resource, action, description)
			VALUES (?, ?, ?, ?)`,
			id, perm.Resource, perm.Action, perm.Description,
		)
		if err != nil {
			return fmt.Errorf("inserting permission %s:%s: %w", perm.Resource, perm.Action, err)
		}
	}

	// Insert predefined roles and their permissions
	for _, roleDef := range PredefinedRoles {
		roleID := uuid.New().String()
		_, err := tx.Exec(`
			INSERT OR IGNORE INTO roles (id, name, description, is_builtin, created_at, updated_at)
			VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
			roleID, roleDef.Name, roleDef.Description, true,
		)
		if err != nil {
			return fmt.Errorf("inserting role %s: %w", roleDef.Name, err)
		}

		// Get the actual role ID if it already existed
		err = tx.QueryRow(`SELECT id FROM roles WHERE name = ?`, roleDef.Name).Scan(&roleID)
		if err != nil {
			return fmt.Errorf("getting role ID for %s: %w", roleDef.Name, err)
		}

		// Assign permissions to the role
		for _, perm := range roleDef.Permissions {
			var permID string
			err := tx.QueryRow(`
				SELECT id FROM permissions WHERE resource = ? AND action = ?`,
				perm.Resource, perm.Action,
			).Scan(&permID)
			if err != nil {
				continue // Permission might not exist yet
			}

			_, err = tx.Exec(`
				INSERT OR IGNORE INTO role_permissions (role_id, permission_id, created_at)
				VALUES (?, ?, datetime('now'))`,
				roleID, permID,
			)
			if err != nil {
				return fmt.Errorf("assigning permission %s:%s to role %s: %w",
					perm.Resource, perm.Action, roleDef.Name, err)
			}
		}
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing transaction: %w", err)
	}

	return nil
}

// CheckPermission checks if a user has a specific permission.
func (rm *RBACManager) CheckPermission(userID, resource, action string) (bool, error) {
	// Check permissions from directly assigned roles (both builtin and custom).
	var count int
	err := rm.db.QueryRow(`
		SELECT COUNT(*)
		FROM user_roles ur
		JOIN role_permissions rp ON ur.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE ur.user_id = ? AND p.resource = ? AND p.action = ?`,
		userID, resource, action,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking direct role permissions: %w", err)
	}
	if count > 0 {
		return true, nil
	}

	// Check permissions from group roles.
	err = rm.db.QueryRow(`
		SELECT COUNT(*)
		FROM user_groups ug
		JOIN group_roles gr ON ug.group_id = gr.group_id
		JOIN role_permissions rp ON gr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE ug.user_id = ? AND p.resource = ? AND p.action = ?`,
		userID, resource, action,
	).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking group permissions: %w", err)
	}

	return count > 0, nil
}

// GetUserPermissions returns all permissions for a user (from both direct and group roles).
func (rm *RBACManager) GetUserPermissions(userID string) ([]Permission, error) {
	// Get permissions from direct roles
	rows, err := rm.db.Query(`
		SELECT DISTINCT p.id, p.resource, p.action, p.description
		FROM user_roles ur
		JOIN role_permissions rp ON ur.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE ur.user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user permissions: %w", err)
	}
	defer rows.Close()

	permMap := make(map[string]Permission)
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Resource, &p.Action, &p.Description); err != nil {
			return nil, fmt.Errorf("scanning permission: %w", err)
		}
		permMap[p.Resource+":"+p.Action] = p
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user permissions: %w", err)
	}

	// Get permissions from group roles
	rows2, err := rm.db.Query(`
		SELECT DISTINCT p.id, p.resource, p.action, p.description
		FROM user_groups ug
		JOIN group_roles gr ON ug.group_id = gr.group_id
		JOIN role_permissions rp ON gr.role_id = rp.role_id
		JOIN permissions p ON rp.permission_id = p.id
		WHERE ug.user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying group permissions: %w", err)
	}
	defer rows2.Close()

	for rows2.Next() {
		var p Permission
		if err := rows2.Scan(&p.ID, &p.Resource, &p.Action, &p.Description); err != nil {
			return nil, fmt.Errorf("scanning group permission: %w", err)
		}
		permMap[p.Resource+":"+p.Action] = p
	}
	if err := rows2.Err(); err != nil {
		return nil, fmt.Errorf("iterating group permissions: %w", err)
	}

	permissions := make([]Permission, 0, len(permMap))
	for _, p := range permMap {
		permissions = append(permissions, p)
	}

	return permissions, nil
}

// AssignRole assigns a role to a user.
func (rm *RBACManager) AssignRole(userID, roleID string) error {
	_, err := rm.db.Exec(`
		INSERT OR IGNORE INTO user_roles (user_id, role_id, created_at)
		VALUES (?, ?, datetime('now'))`,
		userID, roleID,
	)
	if err != nil {
		return fmt.Errorf("assigning role: %w", err)
	}
	return nil
}

// RemoveRole removes a role from a user.
func (rm *RBACManager) RemoveRole(userID, roleID string) error {
	_, err := rm.db.Exec(`DELETE FROM user_roles WHERE user_id = ? AND role_id = ?`, userID, roleID)
	if err != nil {
		return fmt.Errorf("removing role: %w", err)
	}
	return nil
}

// GetUserRoles returns all roles assigned to a user.
func (rm *RBACManager) GetUserRoles(userID string) ([]Role, error) {
	rows, err := rm.db.Query(`
		SELECT r.id, r.name, r.description, r.is_builtin, r.created_at, r.updated_at
		FROM user_roles ur
		JOIN roles r ON ur.role_id = r.id
		WHERE ur.user_id = ?`,
		userID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying user roles: %w", err)
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var r Role
		var createdAt, updatedAt string
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.IsBuiltin, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning role: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		roles = append(roles, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating user roles: %w", err)
	}

	return roles, nil
}

// GetRole retrieves a role by ID. Permissions are loaded eagerly; any error
// loading permissions is returned to the caller (previously the error was
// silently swallowed, leading to roles being returned without permission
// data after a transient database failure).
func (rm *RBACManager) GetRole(roleID string) (*Role, error) {
	var r Role
	var createdAt, updatedAt string
	err := rm.db.QueryRow(`
		SELECT id, name, description, is_builtin, created_at, updated_at
		FROM roles WHERE id = ?`, roleID,
	).Scan(&r.ID, &r.Name, &r.Description, &r.IsBuiltin, &createdAt, &updatedAt)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("role not found")
	}
	if err != nil {
		return nil, fmt.Errorf("querying role: %w", err)
	}
	r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)

	// Load permissions for this role. Surface the error rather than
	// dropping it on the floor.
	perms, err := rm.GetRolePermissions(roleID)
	if err != nil {
		return nil, fmt.Errorf("loading role permissions: %w", err)
	}
	r.Permissions = perms

	return &r, nil
}

// GetRolePermissions returns all permissions for a role.
func (rm *RBACManager) GetRolePermissions(roleID string) ([]Permission, error) {
	rows, err := rm.db.Query(`
		SELECT p.id, p.resource, p.action, p.description
		FROM role_permissions rp
		JOIN permissions p ON rp.permission_id = p.id
		WHERE rp.role_id = ?`,
		roleID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying role permissions: %w", err)
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Resource, &p.Action, &p.Description); err != nil {
			return nil, fmt.Errorf("scanning permission: %w", err)
		}
		permissions = append(permissions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating role permissions: %w", err)
	}

	return permissions, nil
}

// CreateRole creates a new custom role.
func (rm *RBACManager) CreateRole(name, description string) (*Role, error) {
	// Check if a role with the same name already exists.
	var existingID string
	err := rm.db.QueryRow(`SELECT id FROM roles WHERE name = ?`, name).Scan(&existingID)
	if err == nil {
		return nil, fmt.Errorf("role with name %q already exists", name)
	}
	if err != sql.ErrNoRows {
		return nil, fmt.Errorf("checking role name: %w", err)
	}

	id := uuid.New().String()
	_, err = rm.db.Exec(`
		INSERT INTO roles (id, name, description, is_builtin, created_at, updated_at)
		VALUES (?, ?, ?, ?, datetime('now'), datetime('now'))`,
		id, name, description, false,
	)
	if err != nil {
		return nil, fmt.Errorf("creating role: %w", err)
	}

	return &Role{
		ID:          id,
		Name:        name,
		Description: description,
		IsBuiltin:   false,
	}, nil
}

// UpdateRole updates a custom role.
func (rm *RBACManager) UpdateRole(roleID, name, description string) error {
	result, err := rm.db.Exec(`
		UPDATE roles SET name = ?, description = ?, updated_at = datetime('now')
		WHERE id = ? AND is_builtin = 0`,
		name, description, roleID,
	)
	if err != nil {
		return fmt.Errorf("updating role: %w", err)
	}
	rowsAffected, _ := result.RowsAffected()
	if rowsAffected == 0 {
		return fmt.Errorf("role not found or is a built-in role")
	}
	return nil
}

// DeleteRole deletes a custom role and all related records.
func (rm *RBACManager) DeleteRole(roleID string) error {
	tx, err := rm.db.Begin()
	if err != nil {
		return fmt.Errorf("starting transaction: %w", err)
	}
	defer tx.Rollback()

	// Check if the role is a built-in role before deleting anything.
	var isBuiltin bool
	err = tx.QueryRow(`SELECT is_builtin FROM roles WHERE id = ?`, roleID).Scan(&isBuiltin)
	if err != nil {
		return fmt.Errorf("role not found")
	}
	if isBuiltin {
		return fmt.Errorf("cannot delete built-in role")
	}

	// Delete role_permissions
	if _, err := tx.Exec(`DELETE FROM role_permissions WHERE role_id = ?`, roleID); err != nil {
		return fmt.Errorf("deleting role permissions: %w", err)
	}
	// Delete user_roles
	if _, err := tx.Exec(`DELETE FROM user_roles WHERE role_id = ?`, roleID); err != nil {
		return fmt.Errorf("deleting user roles: %w", err)
	}
	// Delete group_roles
	if _, err := tx.Exec(`DELETE FROM group_roles WHERE role_id = ?`, roleID); err != nil {
		return fmt.Errorf("deleting group roles: %w", err)
	}
	// Delete the role itself
	if _, err := tx.Exec(`DELETE FROM roles WHERE id = ?`, roleID); err != nil {
		return fmt.Errorf("deleting role: %w", err)
	}
	return tx.Commit()
}

// ListRoles returns all roles.
func (rm *RBACManager) ListRoles() ([]Role, error) {
	rows, err := rm.db.Query(`
		SELECT id, name, description, is_builtin, created_at, updated_at
		FROM roles ORDER BY name`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying roles: %w", err)
	}
	defer rows.Close()

	var roles []Role
	for rows.Next() {
		var r Role
		var createdAt, updatedAt string
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &r.IsBuiltin, &createdAt, &updatedAt); err != nil {
			return nil, fmt.Errorf("scanning role: %w", err)
		}
		r.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
		r.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAt)
		roles = append(roles, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating roles: %w", err)
	}

	return roles, nil
}

// ListPermissions returns all permissions.
func (rm *RBACManager) ListPermissions() ([]Permission, error) {
	rows, err := rm.db.Query(`
		SELECT id, resource, action, description FROM permissions ORDER BY resource, action`,
	)
	if err != nil {
		return nil, fmt.Errorf("querying permissions: %w", err)
	}
	defer rows.Close()

	var permissions []Permission
	for rows.Next() {
		var p Permission
		if err := rows.Scan(&p.ID, &p.Resource, &p.Action, &p.Description); err != nil {
			return nil, fmt.Errorf("scanning permission: %w", err)
		}
		permissions = append(permissions, p)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating permissions: %w", err)
	}

	return permissions, nil
}

// AssignPermissionToRole assigns a permission to a role.
func (rm *RBACManager) AssignPermissionToRole(roleID, permissionID string) error {
	_, err := rm.db.Exec(`
		INSERT OR IGNORE INTO role_permissions (role_id, permission_id, created_at)
		VALUES (?, ?, datetime('now'))`,
		roleID, permissionID,
	)
	if err != nil {
		return fmt.Errorf("assigning permission to role: %w", err)
	}
	return nil
}

// RemovePermissionFromRole removes a permission from a role.
func (rm *RBACManager) RemovePermissionFromRole(roleID, permissionID string) error {
	_, err := rm.db.Exec(`
		DELETE FROM role_permissions WHERE role_id = ? AND permission_id = ?`,
		roleID, permissionID,
	)
	if err != nil {
		return fmt.Errorf("removing permission from role: %w", err)
	}
	return nil
}

// AssignRoleToGroup assigns a role to a group.
func (rm *RBACManager) AssignRoleToGroup(groupID, roleID string) error {
	_, err := rm.db.Exec(`
		INSERT OR IGNORE INTO group_roles (group_id, role_id, created_at)
		VALUES (?, ?, datetime('now'))`,
		groupID, roleID,
	)
	if err != nil {
		return fmt.Errorf("assigning role to group: %w", err)
	}
	return nil
}

// RemoveRoleFromGroup removes a role from a group.
func (rm *RBACManager) RemoveRoleFromGroup(groupID, roleID string) error {
	_, err := rm.db.Exec(`DELETE FROM group_roles WHERE group_id = ? AND role_id = ?`, groupID, roleID)
	if err != nil {
		return fmt.Errorf("removing role from group: %w", err)
	}
	return nil
}
