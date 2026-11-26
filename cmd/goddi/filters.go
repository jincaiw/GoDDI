package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/jincaiw/GoDDI/internal/api/handler"
	"github.com/jincaiw/GoDDI/internal/dns/filter"
	"github.com/jincaiw/GoDDI/internal/dns/forwarder"
)

// loadFilterData loads block lists, allow rules, and client policies from the database.
func loadFilterData(db *sql.DB, filterEngine *filter.FilterEngine) error {
	if err := handler.LoadBlockListsFromDB(db, filterEngine); err != nil {
		return fmt.Errorf("loading block lists: %w", err)
	}
	if err := handler.LoadAllowRulesFromDB(db, filterEngine); err != nil {
		return fmt.Errorf("loading allow rules: %w", err)
	}
	if err := handler.LoadClientPoliciesFromDB(db, filterEngine); err != nil {
		return fmt.Errorf("loading client policies: %w", err)
	}
	return nil
}

// loadForwardersFromDB loads forwarders from the database into the forwarder group.
func loadForwardersFromDB(db *sql.DB, fwdGroup *forwarder.ForwarderGroup) error {
	rows, err := db.Query("SELECT id, name, protocol, address, enabled, priority FROM dns_forwarders")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, name, protocol, address string
		var enabled bool
		var priority int
		if err := rows.Scan(&id, &name, &protocol, &address, &enabled, &priority); err != nil {
			slog.Warn("failed to scan forwarder row", "error", err)
			continue
		}

		fwd := &forwarder.Forwarder{
			ID:       id,
			Name:     name,
			Protocol: protocol,
			Address:  address,
			Enabled:  enabled,
			Priority: priority,
		}
		fwd.SetHealthy(true)
		if err := fwdGroup.AddForwarder(fwd); err != nil {
			slog.Warn("skipping invalid forwarder from database", "name", name, "error", err)
		}
	}

	slog.Info("loaded forwarders from database", "count", len(fwdGroup.GetForwarders()))
	return nil
}

// loadConditionalForwardersFromDB loads conditional forwarders from the database.
func loadConditionalForwardersFromDB(db *sql.DB, condManager *forwarder.ConditionalForwarderManager, fwdGroup *forwarder.ForwarderGroup) error {
	rows, err := db.Query("SELECT id, domain, forwarder_ids, enabled FROM dns_conditional_forwarders")
	if err != nil {
		return err
	}
	defer rows.Close()

	for rows.Next() {
		var id, domain, forwarderIDsJSON string
		var enabled bool
		if err := rows.Scan(&id, &domain, &forwarderIDsJSON, &enabled); err != nil {
			continue
		}

		// Parse forwarder IDs from JSON.
		var fwdIDs []string
		if forwarderIDsJSON != "" && forwarderIDsJSON != "null" {
			if err := json.Unmarshal([]byte(forwarderIDsJSON), &fwdIDs); err != nil {
				slog.Warn("failed to parse forwarder IDs JSON", "id", id, "error", err)
			}
		}

		cf := &forwarder.ConditionalForwarder{
			ID:           id,
			Domain:       domain,
			ForwarderIDs: fwdIDs,
			Enabled:      enabled,
		}
		condManager.AddConditional(cf)
	}

	// Rebuild forwarder groups for all conditional forwarders.
	allFwds := fwdGroup.GetForwarders()
	condManager.RebuildGroups(allFwds)

	slog.Info("loaded conditional forwarders from database", "count", len(condManager.GetConditionals()))
	return nil
}
