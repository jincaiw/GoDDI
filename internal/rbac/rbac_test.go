package rbac

import (
	"database/sql"
	"testing"

	_ "modernc.org/sqlite"
)

func setupTestDB(t *testing.T) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory database: %v", err)
	}

	// Create required tables
	schema := `
	CREATE TABLE IF NOT EXISTS permissions (
		id TEXT PRIMARY KEY,
		resource TEXT NOT NULL,
		action TEXT NOT NULL,
		description TEXT,
		UNIQUE(resource, action)
	);
	CREATE TABLE IF NOT EXISTS roles (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL UNIQUE,
		description TEXT,
		is_builtin INTEGER NOT NULL DEFAULT 0,
		created_at DATETIME NOT NULL,
		updated_at DATETIME NOT NULL
	);
	CREATE TABLE IF NOT EXISTS role_permissions (
		role_id TEXT NOT NULL,
		permission_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (role_id, permission_id)
	);
	CREATE TABLE IF NOT EXISTS user_roles (
		user_id TEXT NOT NULL,
		role_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (user_id, role_id)
	);
	CREATE TABLE IF NOT EXISTS user_groups (
		user_id TEXT NOT NULL,
		group_id TEXT NOT NULL,
		PRIMARY KEY (user_id, group_id)
	);
	CREATE TABLE IF NOT EXISTS group_roles (
		group_id TEXT NOT NULL,
		role_id TEXT NOT NULL,
		created_at DATETIME NOT NULL,
		PRIMARY KEY (group_id, role_id)
	);
	`
	_, err = db.Exec(schema)
	if err != nil {
		t.Fatalf("failed to create schema: %v", err)
	}

	return db
}

func TestNewRBACManager(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	if rm == nil {
		t.Fatal("NewRBACManager() returned nil")
	}
}

func TestInitializePredefinedData(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	err := rm.InitializePredefinedData()
	if err != nil {
		t.Fatalf("InitializePredefinedData() error = %v", err)
	}

	// Verify permissions were created
	perms, err := rm.ListPermissions()
	if err != nil {
		t.Fatalf("ListPermissions() error = %v", err)
	}
	if len(perms) != len(PredefinedPermissions) {
		t.Errorf("ListPermissions() returned %d, want %d", len(perms), len(PredefinedPermissions))
	}

	// Verify roles were created
	roles, err := rm.ListRoles()
	if err != nil {
		t.Fatalf("ListRoles() error = %v", err)
	}
	if len(roles) != len(PredefinedRoles) {
		t.Errorf("ListRoles() returned %d, want %d", len(roles), len(PredefinedRoles))
	}
}

func TestInitializePredefinedData_Idempotent(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)

	// Call twice - should not error
	err := rm.InitializePredefinedData()
	if err != nil {
		t.Fatalf("first InitializePredefinedData() error = %v", err)
	}
	err = rm.InitializePredefinedData()
	if err != nil {
		t.Fatalf("second InitializePredefinedData() error = %v", err)
	}

	// Should still have the same number of permissions
	perms, _ := rm.ListPermissions()
	if len(perms) != len(PredefinedPermissions) {
		t.Errorf("duplicate init: ListPermissions() returned %d, want %d", len(perms), len(PredefinedPermissions))
	}
}

func TestCreateRole(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)

	role, err := rm.CreateRole("custom-role", "A custom role")
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if role.Name != "custom-role" {
		t.Errorf("role name = %q, want %q", role.Name, "custom-role")
	}
	if role.IsBuiltin {
		t.Error("custom role should not be builtin")
	}
}

func TestGetRole(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	rm.InitializePredefinedData()

	// Get roles list to find an ID
	roles, _ := rm.ListRoles()
	if len(roles) == 0 {
		t.Fatal("no roles found")
	}

	role, err := rm.GetRole(roles[0].ID)
	if err != nil {
		t.Fatalf("GetRole() error = %v", err)
	}
	if role.Name != roles[0].Name {
		t.Errorf("role name = %q, want %q", role.Name, roles[0].Name)
	}
}

func TestGetRole_NotFound(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)

	_, err := rm.GetRole("nonexistent-id")
	if err == nil {
		t.Error("GetRole() should return error for nonexistent role")
	}
}

func TestUpdateRole(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)

	// Create a custom role
	role, _ := rm.CreateRole("custom-role", "Original description")

	// Update it
	err := rm.UpdateRole(role.ID, "updated-role", "Updated description")
	if err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}

	// Verify update
	updated, _ := rm.GetRole(role.ID)
	if updated.Name != "updated-role" {
		t.Errorf("role name = %q, want %q", updated.Name, "updated-role")
	}
}

func TestDeleteRole(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)

	role, _ := rm.CreateRole("custom-role", "To be deleted")

	err := rm.DeleteRole(role.ID)
	if err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}

	_, err = rm.GetRole(role.ID)
	if err == nil {
		t.Error("GetRole() should return error after deletion")
	}
}

func TestDeleteRole_Builtin(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	rm.InitializePredefinedData()

	// Try to delete a builtin role - should not actually delete
	roles, _ := rm.ListRoles()
	var adminRole *Role
	for _, r := range roles {
		if r.Name == "admin" {
			adminRole = &r
			break
		}
	}
	if adminRole == nil {
		t.Fatal("admin role not found")
	}

	err := rm.DeleteRole(adminRole.ID)
	// Deleting a builtin role should now return an error
	if err == nil {
		t.Fatal("DeleteRole() for builtin should return error")
	}

	// Role should still exist
	_, err = rm.GetRole(adminRole.ID)
	if err != nil {
		t.Error("builtin role should still exist after delete attempt")
	}
}

func TestAssignAndCheckPermission(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	rm.InitializePredefinedData()

	// Create a custom role
	role, _ := rm.CreateRole("dns-reader", "DNS Reader")

	// Get dns:read permission
	perms, _ := rm.ListPermissions()
	var dnsReadPerm *Permission
	for _, p := range perms {
		if p.Resource == "dns" && p.Action == "read" {
			dnsReadPerm = &p
			break
		}
	}
	if dnsReadPerm == nil {
		t.Fatal("dns:read permission not found")
	}

	// Assign permission to role
	err := rm.AssignPermissionToRole(role.ID, dnsReadPerm.ID)
	if err != nil {
		t.Fatalf("AssignPermissionToRole() error = %v", err)
	}

	// Assign role to user
	err = rm.AssignRole("user-1", role.ID)
	if err != nil {
		t.Fatalf("AssignRole() error = %v", err)
	}

	// Check permission
	hasPermission, err := rm.CheckPermission("user-1", "dns", "read")
	if err != nil {
		t.Fatalf("CheckPermission() error = %v", err)
	}
	if !hasPermission {
		t.Error("user-1 should have dns:read permission")
	}

	// Check denied permission
	hasPermission, err = rm.CheckPermission("user-1", "dns", "write")
	if err != nil {
		t.Fatalf("CheckPermission() error = %v", err)
	}
	if hasPermission {
		t.Error("user-1 should NOT have dns:write permission")
	}
}

func TestCheckPermission_NoRole(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	rm.InitializePredefinedData()

	hasPermission, err := rm.CheckPermission("user-no-roles", "dns", "read")
	if err != nil {
		t.Fatalf("CheckPermission() error = %v", err)
	}
	if hasPermission {
		t.Error("user without roles should not have any permissions")
	}
}

func TestRemoveRole(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	rm.InitializePredefinedData()

	// Get admin role
	roles, _ := rm.ListRoles()
	var adminRole *Role
	for _, r := range roles {
		if r.Name == "admin" {
			adminRole = &r
			break
		}
	}

	// Assign and then remove
	rm.AssignRole("user-1", adminRole.ID)
	rm.RemoveRole("user-1", adminRole.ID)

	// User should no longer have admin permissions
	hasPermission, _ := rm.CheckPermission("user-1", "dns", "read")
	if hasPermission {
		t.Error("user should not have permission after role removal")
	}
}

func TestGetUserRoles(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	rm.InitializePredefinedData()

	roles, _ := rm.ListRoles()
	var adminRole *Role
	for _, r := range roles {
		if r.Name == "admin" {
			adminRole = &r
			break
		}
	}

	rm.AssignRole("user-1", adminRole.ID)

	userRoles, err := rm.GetUserRoles("user-1")
	if err != nil {
		t.Fatalf("GetUserRoles() error = %v", err)
	}
	if len(userRoles) != 1 {
		t.Errorf("GetUserRoles() returned %d roles, want 1", len(userRoles))
	}
	if userRoles[0].Name != "admin" {
		t.Errorf("role name = %q, want %q", userRoles[0].Name, "admin")
	}
}

func TestGetUserPermissions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	rm.InitializePredefinedData()

	// Assign admin role to user
	roles, _ := rm.ListRoles()
	var adminRole *Role
	for _, r := range roles {
		if r.Name == "admin" {
			adminRole = &r
			break
		}
	}
	rm.AssignRole("user-1", adminRole.ID)

	perms, err := rm.GetUserPermissions("user-1")
	if err != nil {
		t.Fatalf("GetUserPermissions() error = %v", err)
	}
	// Admin should have all permissions
	if len(perms) != len(PredefinedPermissions) {
		t.Errorf("admin user should have %d permissions, got %d", len(PredefinedPermissions), len(perms))
	}
}

func TestGetRolePermissions(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	rm.InitializePredefinedData()

	roles, _ := rm.ListRoles()
	var viewerRole *Role
	for _, r := range roles {
		if r.Name == "viewer" {
			viewerRole = &r
			break
		}
	}

	perms, err := rm.GetRolePermissions(viewerRole.ID)
	if err != nil {
		t.Fatalf("GetRolePermissions() error = %v", err)
	}

	// Viewer should have read-only permissions
	expectedCount := 10 // as defined in PredefinedRoles
	if len(perms) != expectedCount {
		t.Errorf("viewer role should have %d permissions, got %d", expectedCount, len(perms))
	}

	// All viewer permissions should be read-only
	for _, p := range perms {
		if p.Action != "read" {
			t.Errorf("viewer should only have read permissions, got %s:%s", p.Resource, p.Action)
		}
	}
}

func TestRemovePermissionFromRole(t *testing.T) {
	db := setupTestDB(t)
	defer db.Close()

	rm := NewRBACManager(db)
	rm.InitializePredefinedData()

	// Create a custom role and assign a permission
	role, _ := rm.CreateRole("limited", "Limited Role")
	perms, _ := rm.ListPermissions()
	rm.AssignPermissionToRole(role.ID, perms[0].ID)

	// Remove the permission
	err := rm.RemovePermissionFromRole(role.ID, perms[0].ID)
	if err != nil {
		t.Fatalf("RemovePermissionFromRole() error = %v", err)
	}

	// Verify permission was removed
	rolePerms, _ := rm.GetRolePermissions(role.ID)
	if len(rolePerms) != 0 {
		t.Errorf("role should have 0 permissions after removal, got %d", len(rolePerms))
	}
}

func TestPredefinedPermissions_Count(t *testing.T) {
	t.Parallel()

	if len(PredefinedPermissions) == 0 {
		t.Error("PredefinedPermissions should not be empty")
	}

	// Check that we have expected categories
	resources := make(map[string]bool)
	for _, p := range PredefinedPermissions {
		resources[p.Resource] = true
	}
	expectedResources := []string{"dns", "dhcp", "ipam", "user", "role", "settings", "audit", "backup", "token"}
	for _, r := range expectedResources {
		if !resources[r] {
			t.Errorf("missing resource %q in PredefinedPermissions", r)
		}
	}
}

func TestPredefinedRoles_Count(t *testing.T) {
	t.Parallel()

	if len(PredefinedRoles) != 3 {
		t.Errorf("PredefinedRoles count = %d, want 3", len(PredefinedRoles))
	}

	roleNames := make(map[string]bool)
	for _, r := range PredefinedRoles {
		roleNames[r.Name] = true
	}
	for _, name := range []string{"admin", "operator", "viewer"} {
		if !roleNames[name] {
			t.Errorf("missing role %q in PredefinedRoles", name)
		}
	}
}
