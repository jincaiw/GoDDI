package handler

import (
	"database/sql"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/jasonwa/goddi/internal/configver"
	"github.com/jasonwa/goddi/internal/rbac"
)

// recordResourceCreation writes a resource's creation as its first revision.
//
// It is called by the create endpoints of the governed resource types, after
// the rows are in place. Creation is otherwise outside the version log -- the
// publish endpoint refuses a resource that does not exist yet, and it has to --
// so without this the first revision records the first edit, the state the
// resource was born in is unanswerable, and no rollback can return to it.
//
// A failure here does not fail the request. The resource exists and is being
// served; a history that could not be written is a fact to report, not a reason
// to delete what was just created. The alternative -- refusing the create
// because the log write failed -- would make the governance layer a way to
// break resource creation, which is a worse failure than a gap in the log, and
// the gap is visible: the resource has no revision 1.
func recordResourceCreation(r *http.Request, rt configver.ResourceType, id string, content any) {
	svc := ConfigVersionService
	if svc == nil || id == "" {
		return
	}
	encoded, err := json.Marshal(content)
	if err != nil {
		slog.Warn("config publishing: the creation could not be encoded",
			"resource_type", rt, "resource_id", id, "error", err)
		return
	}
	actor := ""
	if r != nil {
		actor = rbac.GetUsername(r.Context())
	}
	if _, err := svc.RecordCreation(configver.RecordCreationRequest{
		ResourceType: rt,
		ResourceID:   id,
		Content:      encoded,
		Actor:        actor,
	}); err != nil {
		slog.Warn("config publishing: the creation was not recorded in the version log",
			"resource_type", rt, "resource_id", id, "error", err)
	}
}

// recordRecordSetCreation records the (possibly empty) record set a new zone
// was born with, so that the zone's record history starts at its creation
// rather than at the first record somebody adds.
func recordRecordSetCreation(r *http.Request, db *sql.DB, zoneID string) {
	if ConfigVersionService == nil || db == nil || zoneID == "" {
		return
	}
	content, err := configver.ReadContent(db, configver.ResourceDNSRecords, zoneID)
	if err != nil {
		slog.Warn("config publishing: the new zone's record set could not be read",
			"zone_id", zoneID, "error", err)
		return
	}
	recordResourceCreation(r, configver.ResourceDNSRecords, zoneID, content)
}
