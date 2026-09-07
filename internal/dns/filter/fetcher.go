package filter

import (
	"bufio"
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

// fetchMaxBodySize caps a block list download at 64MB to prevent memory
// amplification from a hostile or misconfigured URL.
const fetchMaxBodySize = 64 << 20

// fetchHTTPTimeout bounds a single block list download.
const fetchHTTPTimeout = 60 * time.Second

// defaultFetchInterval is how often external block lists are refreshed
// when no explicit interval has been configured.
const defaultFetchInterval = 24 * time.Hour

// domainLabelRegex validates a domain label before it is accepted as a
// block rule pattern. Hosts-file style entries are reduced to the domain
// before this check.
var domainLabelRegex = regexp.MustCompile(`^[a-zA-Z0-9_]([a-zA-Z0-9_-]*[a-zA-Z0-9_])?(\.[a-zA-Z0-9_]([a-zA-Z0-9_-]*[a-zA-Z0-9_])?)+$`)

// BlockListFetcher periodically downloads external block list URLs and
// reloads their rules into the BlockListManager. Fetch results (status,
// error, timestamp) are persisted to the dns_block_lists table so the Web
// console can surface them.
type BlockListFetcher struct {
	db  *sql.DB
	mgr *BlockListManager

	client *http.Client

	// interval is how stale a list may be before it is re-fetched.
	// It is guarded by mu and hot-updatable via SetInterval.
	mu       sync.Mutex
	interval time.Duration
}

// NewBlockListFetcher creates a new fetcher. Call Run in a goroutine to
// start the periodic refresh loop.
func NewBlockListFetcher(db *sql.DB, mgr *BlockListManager) *BlockListFetcher {
	return &BlockListFetcher{
		db:  db,
		mgr: mgr,
		client: &http.Client{
			Timeout: fetchHTTPTimeout,
			// Blocklist URLs are operator supplied but reachable by any
			// account holding dns:write. Refuse redirects and reject
			// loopback/link-local/private targets so the fetcher cannot be
			// turned into an SSRF probe against the host or the metadata
			// service (169.254.169.254).
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return http.ErrUseLastResponse
			},
			Transport: &http.Transport{
				Proxy: http.ProxyFromEnvironment,
				DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
					host, port, err := net.SplitHostPort(addr)
					if err != nil {
						return nil, fmt.Errorf("invalid address %q: %w", addr, err)
					}
					if err := assertPublicHost(ctx, host); err != nil {
						return nil, err
					}
					d := &net.Dialer{Timeout: 10 * time.Second}
					return d.DialContext(ctx, network, net.JoinHostPort(host, port))
				},
			},
		},
		interval: defaultFetchInterval,
	}
}

// SetInterval changes the refresh interval (hot-updatable via settings).
func (f *BlockListFetcher) SetInterval(d time.Duration) {
	if d <= 0 {
		return
	}
	f.mu.Lock()
	f.interval = d
	f.mu.Unlock()
}

// Run starts the periodic refresh loop. It blocks until ctx is cancelled.
// The first pass runs immediately after a short delay so startup is not
// blocked by network I/O.
func (f *BlockListFetcher) Run(ctx context.Context) {
	// Give the process a moment to finish booting before the first fetch.
	select {
	case <-time.After(15 * time.Second):
	case <-ctx.Done():
		return
	}

	f.refreshDueLists(ctx)

	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			f.refreshDueLists(ctx)
		}
	}
}

// refreshDueLists fetches every enabled external list whose last successful
// fetch is older than the configured interval.
func (f *BlockListFetcher) refreshDueLists(ctx context.Context) {
	f.mu.Lock()
	interval := f.interval
	f.mu.Unlock()

	for _, list := range f.mgr.ListLists() {
		if !list.Enabled || list.Type != "external" || list.URL == "" {
			continue
		}

		var lastFetch time.Time
		if list.LastFetchAt != "" {
			if t, err := time.Parse(time.RFC3339, list.LastFetchAt); err == nil {
				lastFetch = t
			}
		}
		// A previously failed list is retried on the next pass (max once
		// per 15 min tick) rather than waiting the full interval.
		if list.LastFetchStatus == "success" && time.Since(lastFetch) < interval {
			continue
		}

		if err := f.FetchList(ctx, list.ID); err != nil {
			slog.Warn("blocklist_fetch: refresh failed", "list", list.Name, "error", err)
		}
	}
}

// FetchList downloads, parses and applies the rules of one block list.
// The existing rules for the list are replaced atomically on success; on
// failure the previous rules remain active.
func (f *BlockListFetcher) FetchList(ctx context.Context, listID string) error {
	list, ok := f.mgr.GetList(listID)
	if !ok {
		return fmt.Errorf("block list not found: %s", listID)
	}
	if list.Type != "external" || list.URL == "" {
		return fmt.Errorf("list %s is not an external URL list", listID)
	}

	domains, err := f.download(ctx, list.URL)
	now := time.Now().UTC().Format(time.RFC3339)
	if err != nil {
		f.recordFetchResult(listID, now, "failed", err.Error())
		return fmt.Errorf("downloading block list: %w", err)
	}
	if len(domains) == 0 {
		err := fmt.Errorf("block list contained no usable domains")
		f.recordFetchResult(listID, now, "failed", err.Error())
		return err
	}

	if err := f.applyRules(listID, domains); err != nil {
		f.recordFetchResult(listID, now, "failed", err.Error())
		return err
	}

	f.recordFetchResult(listID, now, "success", "")
	slog.Info("blocklist_fetch: list refreshed", "list", list.Name, "entries", len(domains))
	return nil
}

// AssertPublicURL validates that rawURL is an http(s) URL whose host is not a
// loopback, link-local, unspecified or private address. It is used both when a
// block list is created/updated (fail fast, clear error to the operator) and
// by the fetcher itself, which re-checks on every dial so a DNS-rebinding race
// cannot smuggle a private address past the creation-time check.
func AssertPublicURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parsing URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("URL scheme must be http or https")
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("URL host is required")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return assertPublicHost(ctx, host)
}

// assertPublicHost rejects hosts that resolve to (or are) loopback,
// link-local or private addresses, which are never legitimate blocklist
// sources and are the usual SSRF targets (cloud metadata service, internal
// dashboards). Resolution happens per dial so a rebinding race cannot
// smuggle a private address past the check.
func assertPublicHost(ctx context.Context, host string) error {
	if ip := net.ParseIP(host); ip != nil {
		return assertPublicIP(ip)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return fmt.Errorf("resolving %q: %w", host, err)
	}
	if len(ips) == 0 {
		return fmt.Errorf("no addresses for %q", host)
	}
	for _, ip := range ips {
		if err := assertPublicIP(ip.IP); err != nil {
			return err
		}
	}
	return nil
}

func assertPublicIP(ip net.IP) error {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsUnspecified() || ip.IsInterfaceLocalMulticast() {
		return fmt.Errorf("refusing to fetch from non-public address %s", ip)
	}
	if ip.IsPrivate() {
		return fmt.Errorf("refusing to fetch from private address %s", ip)
	}
	return nil
}

// download fetches the URL and parses it into a domain list. Supported
// formats: one domain per line, hosts-file entries ("0.0.0.0 domain") and
// adblock-style "||domain^" lines. Comments (#, !) and blanks are skipped.
func (f *BlockListFetcher) download(ctx context.Context, rawURL string) ([]string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "GoDDI-BlockListFetcher/1.0")

	resp, err := f.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %s", resp.Status)
	}
	if resp.ContentLength > fetchMaxBodySize {
		return nil, fmt.Errorf("body too large: %d bytes", resp.ContentLength)
	}

	domains := make([]string, 0, 4096)
	seen := make(map[string]struct{}, 4096)

	scanner := bufio.NewScanner(io.LimitReader(resp.Body, fetchMaxBodySize))
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, "!") {
			continue
		}
		line = strings.TrimPrefix(line, "||")
		line = strings.TrimSuffix(line, "^")
		// Hosts-file format: "0.0.0.0 ads.example.com".
		if fields := strings.Fields(line); len(fields) >= 2 {
			first := strings.ToLower(fields[0])
			if first == "0.0.0.0" || first == "127.0.0.1" || first == "::" || first == "::1" {
				line = strings.ToLower(fields[1])
			}
		}
		domain := stripDot(strings.ToLower(strings.TrimSpace(line)))
		if domain == "" || !domainLabelRegex.MatchString(domain) || len(domain) > 253 {
			continue
		}
		if _, dup := seen[domain]; dup {
			continue
		}
		seen[domain] = struct{}{}
		domains = append(domains, domain)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("scanning block list body: %w", err)
	}
	return domains, nil
}

// applyRules replaces the rules of the list in the database and reloads the
// in-memory manager.
func (f *BlockListFetcher) applyRules(listID string, domains []string) error {
	tx, err := f.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec("DELETE FROM dns_block_rules WHERE list_id = ?", listID); err != nil {
		return err
	}

	stmt, err := tx.Prepare(`
		INSERT INTO dns_block_rules (id, list_id, pattern, match_type, response_type, enabled)
		VALUES (?, ?, ?, 'suffix', 'NXDOMAIN', 1)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	rules := make([]MatchRule, 0, len(domains))
	for _, domain := range domains {
		id := uuid.New().String()
		if _, err := stmt.Exec(id, listID, domain); err != nil {
			return err
		}
		rules = append(rules, MatchRule{
			ID:           id,
			ListID:       listID,
			Pattern:      domain,
			MatchType:    "suffix",
			ResponseType: "NXDOMAIN",
			Enabled:      true,
		})
	}

	if _, err := tx.Exec(`
		UPDATE dns_block_lists
		SET entry_count = ?, last_updated = datetime('now'), updated_at = datetime('now')
		WHERE id = ?
	`, len(rules), listID); err != nil {
		return err
	}

	if err := tx.Commit(); err != nil {
		return err
	}

	// Rebuild the in-memory rules for this list from the full DB state.
	f.reloadListRules(listID, rules)
	return nil
}

// reloadListRules merges the new rules for one list into the manager without
// discarding rules that belong to other lists.
func (f *BlockListFetcher) reloadListRules(listID string, rules []MatchRule) {
	m := f.mgr

	m.mu.Lock()
	// Remove old rules of this list from the trie.
	for _, r := range m.rules[listID] {
		m.trie.Remove(r.ID)
	}
	m.rules[listID] = rules
	for i := range rules {
		rules[i].ListID = listID
		if rules[i].MatchType == "regex" && rules[i].compiledRegex == nil {
			if re, err := regexp.Compile(rules[i].Pattern); err == nil {
				rules[i].compiledRegex = re
			}
		}
		if rules[i].Enabled {
			m.trie.Insert(rules[i])
		}
	}
	if list, ok := m.lists[listID]; ok {
		list.EntryCount = len(rules)
	}
	total := m.trie.Size()
	m.mu.Unlock()

	slog.Debug("blocklist_fetch: rules reloaded", "list_id", listID, "total_rules", total)
}

// recordFetchResult persists the fetch outcome on the list row and updates
// the in-memory metadata.
func (f *BlockListFetcher) recordFetchResult(listID, at, status, errMsg string) {
	_, _ = f.db.Exec(`
		UPDATE dns_block_lists SET last_fetch_at = ?, last_fetch_status = ?, last_fetch_error = ?
		WHERE id = ?
	`, at, status, errMsg, listID)

	if list, ok := f.mgr.GetList(listID); ok {
		list.LastFetchAt = at
		list.LastFetchStatus = status
		list.LastFetchError = errMsg
	}
}
