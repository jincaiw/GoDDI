package backup

import (
	"database/sql"
	"encoding/json"
	"fmt"
)

// ExportDNS exports all DNS data as JSON.
func ExportDNS(db *sql.DB) (json.RawMessage, error) {
	data := map[string]interface{}{}

	// Export zones.
	zones, err := exportTable(db, "SELECT id, name, type, enabled, dnssec_enabled, default_ttl, soa_mname, soa_rname, serial, refresh, retry, expire, minimum, transfer_policy, update_policy, created_at, updated_at FROM dns_zones")
	if err != nil {
		return nil, fmt.Errorf("exporting zones: %w", err)
	}
	data["zones"] = zones

	// Export records.
	records, err := exportTable(db, "SELECT id, zone_id, name, type, value, ttl, priority, weight, port, enabled, comment, tags, tag, flag, owner, expires_at, created_at, updated_at FROM dns_records")
	if err != nil {
		return nil, fmt.Errorf("exporting records: %w", err)
	}
	data["records"] = records

	// Export forwarders.
	forwarders, err := exportTable(db, "SELECT id, name, protocol, address, enabled, priority, created_at, updated_at FROM dns_forwarders")
	if err != nil {
		return nil, fmt.Errorf("exporting forwarders: %w", err)
	}
	data["forwarders"] = forwarders

	// Export conditional forwarders.
	condFwd, err := exportTable(db, "SELECT id, domain, forwarder_ids, enabled, created_at, updated_at FROM dns_conditional_forwarders")
	if err != nil {
		return nil, fmt.Errorf("exporting conditional forwarders: %w", err)
	}
	data["conditional_forwarders"] = condFwd

	return json.Marshal(data)
}

// ExportDHCP exports all DHCP data as JSON.
func ExportDHCP(db *sql.DB) (json.RawMessage, error) {
	data := map[string]interface{}{}

	// Export scopes.
	scopes, err := exportTable(db, "SELECT id, name, interface, subnet, start_ip, end_ip, subnet_mask, router, dns_servers, ntp_servers, domain_name, lease_time, max_lease_time, enabled, ping_check_enabled, dns_updates, comment, created_at, updated_at FROM dhcp_scopes")
	if err != nil {
		return nil, fmt.Errorf("exporting scopes: %w", err)
	}
	data["scopes"] = scopes

	// Export options.
	options, err := exportTable(db, "SELECT id, scope_id, reservation_id, code, value, priority, created_at, updated_at FROM dhcp_options")
	if err != nil {
		return nil, fmt.Errorf("exporting options: %w", err)
	}
	data["options"] = options

	// Export reservations.
	reservations, err := exportTable(db, "SELECT id, scope_id, ip_address, mac_address, hostname, description, enabled, created_at, updated_at FROM dhcp_reservations")
	if err != nil {
		return nil, fmt.Errorf("exporting reservations: %w", err)
	}
	data["reservations"] = reservations

	leases, err := exportTable(db, "SELECT id, scope_id, ip_address, mac_address, hostname, client_id, lease_start, lease_end, status, last_seen FROM dhcp_leases")
	if err != nil {
		return nil, fmt.Errorf("exporting leases: %w", err)
	}
	data["leases"] = leases

	return json.Marshal(data)
}

// ExportIPAM exports all IPAM data as JSON.
func ExportIPAM(db *sql.DB) (json.RawMessage, error) {
	data := map[string]interface{}{}

	// Export spaces.
	spaces, err := exportTable(db, "SELECT id, name, description, created_at, updated_at FROM ipam_spaces")
	if err != nil {
		return nil, fmt.Errorf("exporting spaces: %w", err)
	}
	data["spaces"] = spaces

	// Export subnets.
	subnets, err := exportTable(db, "SELECT id, space_id, name, cidr, vlan_id, location, description, created_at, updated_at FROM ipam_subnets")
	if err != nil {
		return nil, fmt.Errorf("exporting subnets: %w", err)
	}
	data["subnets"] = subnets

	// Export addresses.
	addresses, err := exportTable(db, "SELECT id, subnet_id, ip_address, status, mac_address, hostname, dns_record_id, dhcp_lease_id, owner, device, location, description, last_seen, created_at, updated_at FROM ipam_addresses")
	if err != nil {
		return nil, fmt.Errorf("exporting addresses: %w", err)
	}
	data["addresses"] = addresses

	return json.Marshal(data)
}

// ExportSecurity exports security data (block/allow rules, client policies) as JSON.
func ExportSecurity(db *sql.DB) (json.RawMessage, error) {
	data := map[string]interface{}{}

	// Export block lists.
	blockLists, err := exportTable(db, "SELECT id, name, type, url, enabled, last_updated, entry_count, created_at, updated_at FROM dns_block_lists")
	if err != nil {
		return nil, fmt.Errorf("exporting block lists: %w", err)
	}
	data["block_lists"] = blockLists

	// Export block rules.
	blockRules, err := exportTable(db, "SELECT id, list_id, pattern, match_type, response_type, response_data, enabled, created_at FROM dns_block_rules")
	if err != nil {
		return nil, fmt.Errorf("exporting block rules: %w", err)
	}
	data["block_rules"] = blockRules

	// Export allow rules.
	allowRules, err := exportTable(db, "SELECT id, pattern, match_type, enabled, created_at FROM dns_allow_rules")
	if err != nil {
		return nil, fmt.Errorf("exporting allow rules: %w", err)
	}
	data["allow_rules"] = allowRules

	// Export client policies.
	policies, err := exportTable(db, "SELECT id, name, source_cidr, action, block_list_ids, allow_rule_ids, priority, enabled, created_at, updated_at FROM dns_client_policies")
	if err != nil {
		return nil, fmt.Errorf("exporting client policies: %w", err)
	}
	data["client_policies"] = policies

	return json.Marshal(data)
}

// ExportConfig exports system settings as JSON.
func ExportConfig(db *sql.DB) (json.RawMessage, error) {
	data := map[string]interface{}{}

	// Export system settings.
	settings, err := exportTable(db, "SELECT key, value, description, updated_at FROM system_settings")
	if err != nil {
		return nil, fmt.Errorf("exporting settings: %w", err)
	}
	data["settings"] = settings

	return json.Marshal(data)
}

// exportTable is a generic helper that exports all rows from a query as a slice of maps.
func exportTable(db *sql.DB, query string) ([]map[string]interface{}, error) {
	rows, err := db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}

	result := make([]map[string]interface{}, 0)
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			return nil, fmt.Errorf("scanning export row: %w", err)
		}

		row := make(map[string]interface{})
		for i, col := range columns {
			val := values[i]
			// Handle nil values.
			if val == nil {
				row[col] = ""
				continue
			}
			// Convert []byte to string for JSON compatibility.
			if b, ok := val.([]byte); ok {
				row[col] = string(b)
			} else {
				row[col] = val
			}
		}
		result = append(result, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating export rows: %w", err)
	}

	return result, nil
}
