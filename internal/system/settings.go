package system

import (
	"database/sql"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"
)

// Setting represents a system setting. The optional Type field tells the
// frontend which input control to render (bool/int/string), removing the
// need for hard-coded key lists on the client.
type Setting struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	Description string `json:"description"`
	Type        string `json:"type,omitempty"`
	UpdatedAt   string `json:"updated_at"`
}

// Predefined settings with their default values.
// `Type` is one of "bool", "int", or "string" and is used by the frontend
// to pick the right input control. Keep in sync with the frontend's
// SettingType union.
var defaultSettings = map[string]settingDefault{
	"server_name":                 {Value: "GoDDI", Description: "服务器名称", Type: "string"},
	"server_language":             {Value: "zh-CN", Description: "界面语言", Type: "string"},
	"server_dark_mode":            {Value: "false", Description: "暗黑模式", Type: "bool"},
	"dns_default_ttl":             {Value: "3600", Description: "DNS 默认 TTL", Type: "int"},
	"dns_recursion":               {Value: "true", Description: "允许递归查询", Type: "bool"},
	"dns_blocking_enabled":        {Value: "true", Description: "DNS 阻断总开关", Type: "bool"},
	"dns_rate_limit_qps":          {Value: "0", Description: "客户端查询限速（QPS，0 为不限）", Type: "int"},
	"dns_blocklist_refresh_hours": {Value: "24", Description: "外部阻断列表刷新间隔（小时）", Type: "int"},
	"dns_ecs_mode":                {Value: "strip", Description: "EDNS Client Subnet 策略（strip/passthrough/add）", Type: "string"},
	"dns_ecs_ipv4_prefix_length":  {Value: "24", Description: "ECS IPv4 前缀长度（add 模式，0-32）", Type: "int"},
	"dns_ecs_ipv6_prefix_length":  {Value: "56", Description: "ECS IPv6 前缀长度（add 模式，0-56）", Type: "int"},
	"dns_cache_serve_stale":       {Value: "true", Description: "过期缓存应答（RFC 8767）", Type: "bool"},
	"dns_cache_stale_ttl":         {Value: "86400", Description: "过期应答保留时长（秒）", Type: "int"},
	"dns_cache_prefetch":          {Value: "false", Description: "热点域名预取（剩余 TTL 低于 10% 时后台刷新）", Type: "bool"},
	"dns_cache_min_ttl":           {Value: "60", Description: "缓存最小 TTL（秒）", Type: "int"},
	"dns_cache_max_ttl":           {Value: "86400", Description: "缓存最大 TTL（秒）", Type: "int"},
	"dns_special_zones":           {Value: "true", Description: "本地特殊域应答（RFC 6303/6761，如 localhost/test/onion）", Type: "bool"},
	"dhcp_lease_time":             {Value: "86400", Description: "DHCP 默认租约时间（秒）", Type: "int"},
	"ipam_ping_check":             {Value: "true", Description: "IPAM Ping 检测", Type: "bool"},
	"ipam_auto_scan":              {Value: "false", Description: "IPAM 自动扫描", Type: "bool"},
	"security_rebinding":          {Value: "true", Description: "DNS 重绑定保护", Type: "bool"},
	"log_retention_days":          {Value: "30", Description: "日志保留天数", Type: "int"},
	"backup_auto_enabled":         {Value: "false", Description: "自动备份", Type: "bool"},
	"backup_auto_schedule":        {Value: "0 2 * * *", Description: "自动备份计划（cron）", Type: "string"},
	"backup_retention":            {Value: "7", Description: "备份保留数量", Type: "int"},
}

type settingDefault struct {
	Value       string
	Description string
	Type        string
}

// Manager manages system settings.
type Manager struct {
	db *sql.DB

	// mu guards the subscriber list.
	mu       sync.RWMutex
	watchers []func(key, value string)
}

// Subscribe registers a callback invoked (asynchronously) after every
// successful setting change. Used by the DNS engine to hot-apply settings
// such as recursion, rebinding protection and rate limits.
func (m *Manager) Subscribe(fn func(key, value string)) {
	m.mu.Lock()
	m.watchers = append(m.watchers, fn)
	m.mu.Unlock()
}

// notify dispatches a changed key to all subscribers. Callbacks run in
// their own goroutine so a slow consumer cannot block the settings API.
func (m *Manager) notify(key, value string) {
	m.mu.RLock()
	watchers := make([]func(key, value string), len(m.watchers))
	copy(watchers, m.watchers)
	m.mu.RUnlock()

	for _, fn := range watchers {
		go fn(key, value)
	}
}

// NewManager creates a new system settings manager.
func NewManager(db *sql.DB) *Manager {
	return &Manager{db: db}
}

// EnsureDefaults inserts default settings if they do not exist.
// It uses a single SELECT to read all existing keys and a single batch
// INSERT to fill in the missing ones, instead of one SELECT and one
// INSERT per default key (the previous N+1 pattern).
func (m *Manager) EnsureDefaults() error {
	// Read the set of keys that already exist in the DB.
	existing := make(map[string]struct{}, len(defaultSettings))
	rows, err := m.db.Query(`SELECT key FROM system_settings`)
	if err != nil {
		return fmt.Errorf("listing existing settings: %w", err)
	}
	for rows.Next() {
		var k string
		if err := rows.Scan(&k); err != nil {
			rows.Close()
			return fmt.Errorf("scanning existing setting key: %w", err)
		}
		existing[k] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("iterating existing settings: %w", err)
	}
	rows.Close()

	// Build a single multi-row INSERT for the missing keys.
	missing := make([]struct {
		key, value, description string
	}, 0, len(defaultSettings))
	for key, def := range defaultSettings {
		if _, ok := existing[key]; !ok {
			missing = append(missing, struct {
				key, value, description string
			}{key, def.Value, def.Description})
		}
	}
	if len(missing) == 0 {
		return nil
	}

	now := time.Now().Format(time.RFC3339)
	placeholders := make([]string, 0, len(missing))
	args := make([]interface{}, 0, len(missing)*4)
	for _, m := range missing {
		placeholders = append(placeholders, "(?, ?, ?, ?)")
		args = append(args, m.key, m.value, m.description, now)
	}
	query := "INSERT INTO system_settings (key, value, description, updated_at) VALUES " +
		strings.Join(placeholders, ", ")
	if _, err := m.db.Exec(query, args...); err != nil {
		return fmt.Errorf("inserting default settings: %w", err)
	}
	return nil
}

// GetSetting returns the value of a system setting.
func (m *Manager) GetSetting(key string) (string, error) {
	var value string
	err := m.db.QueryRow(`SELECT value FROM system_settings WHERE key = ?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		// Return default if available.
		if def, ok := defaultSettings[key]; ok {
			return def.Value, nil
		}
		return "", fmt.Errorf("setting not found: %s", key)
	}
	if err != nil {
		return "", fmt.Errorf("querying setting %s: %w", key, err)
	}
	return value, nil
}

// SetSetting creates or updates a system setting.
func (m *Manager) SetSetting(key, value, description string) error {
	now := time.Now().Format(time.RFC3339)

	_, err := m.db.Exec(
		`INSERT INTO system_settings (key, value, description, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value, description = excluded.description, updated_at = excluded.updated_at`,
		key, value, description, now,
	)
	if err != nil {
		return fmt.Errorf("upserting setting %s: %w", key, err)
	}

	m.notify(key, value)
	return nil
}

// ListSettings returns all system settings.
func (m *Manager) ListSettings() ([]Setting, error) {
	rows, err := m.db.Query(`SELECT key, value, description, updated_at FROM system_settings ORDER BY key`)
	if err != nil {
		return nil, fmt.Errorf("listing settings: %w", err)
	}
	defer rows.Close()

	settings := make([]Setting, 0)
	for rows.Next() {
		var s Setting
		if err := rows.Scan(&s.Key, &s.Value, &s.Description, &s.UpdatedAt); err != nil {
			slog.Warn("failed to scan setting row", "error", err)
			continue
		}
		// Annotate each row with its declared type so the UI can pick the
		// right input control. Defaults are also annotated in EnsureDefaults
		// once they get inserted into the DB.
		if def, ok := defaultSettings[s.Key]; ok {
			s.Type = def.Type
		}
		settings = append(settings, s)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating settings: %w", err)
	}

	// Add any missing defaults.
	existingKeys := make(map[string]bool)
	for _, s := range settings {
		existingKeys[s.Key] = true
	}
	for key, def := range defaultSettings {
		if !existingKeys[key] {
			settings = append(settings, Setting{
				Key:         key,
				Value:       def.Value,
				Description: def.Description,
				Type:        def.Type,
			})
		}
	}

	return settings, nil
}

// DeleteSetting deletes a system setting.
func (m *Manager) DeleteSetting(key string) error {
	_, err := m.db.Exec(`DELETE FROM system_settings WHERE key = ?`, key)
	if err != nil {
		return fmt.Errorf("deleting setting %s: %w", key, err)
	}
	return nil
}

// BatchUpdateSettings updates multiple settings at once.
func (m *Manager) BatchUpdateSettings(settings []Setting) error {
	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	now := time.Now().Format(time.RFC3339)
	for _, s := range settings {
		_, err := tx.Exec(
			`INSERT INTO system_settings (key, value, description, updated_at) VALUES (?, ?, ?, ?)
			 ON CONFLICT(key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
			s.Key, s.Value, s.Description, now,
		)
		if err != nil {
			return fmt.Errorf("upserting setting %s: %w", s.Key, err)
		}
	}

	return tx.Commit()
}
