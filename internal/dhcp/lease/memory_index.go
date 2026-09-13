package lease

import (
	"errors"
	"fmt"
	"net"
	"sync"
)

var ErrNoAvailableAddress = errors.New("no available address")

// AddressPool describes the range and enabled reservations needed by the
// in-memory allocator. It is intentionally a snapshot value: control-plane
// changes replace the pool snapshot rather than mutating an active request.
type scopedIP struct {
	scope string
	ip    string
}

type AddressPool struct {
	ScopeID      string
	StartIP      string
	EndIP        string
	Reservations map[string]bool
}

// MemoryIndex is a read-optimized lease index. It is not a source of durable
// truth: callers must rebuild it from a durable snapshot/WAL before serving
// traffic, and writes must be reflected through Upsert/Remove after commit.
type MemoryIndex struct {
	mu        sync.RWMutex
	byID      map[string]Lease
	byIP      map[string]Lease
	byScopeIP map[scopedIP]Lease
	byMAC     map[string]Lease
	pools     map[string]AddressPool
}

func NewMemoryIndex() *MemoryIndex {
	return &MemoryIndex{
		byID:      make(map[string]Lease),
		byIP:      make(map[string]Lease),
		byScopeIP: make(map[scopedIP]Lease),
		byMAC:     make(map[string]Lease),
		pools:     make(map[string]AddressPool),
	}
}

// Replace atomically rebuilds all indexes from a durable snapshot.
//
// It is retained as a compatibility API for callers that cannot yet surface
// snapshot validation errors. New recovery/startup paths must use
// ReplaceChecked, which leaves the current index untouched on invalid input.
func (i *MemoryIndex) Replace(leases []Lease, pools []AddressPool) {
	byID := make(map[string]Lease, len(leases))
	byIP := make(map[string]Lease, len(leases))
	byScopeIP := make(map[scopedIP]Lease, len(leases))
	byMAC := make(map[string]Lease, len(leases))
	poolMap := make(map[string]AddressPool, len(pools))
	for _, pool := range pools {
		poolMap[pool.ScopeID] = clonePool(pool)
	}
	// Resolve duplicate IDs before deriving secondary indexes. This keeps the
	// indexes consistent with the last snapshot value instead of leaving stale
	// IP/MAC entries for an earlier value with the same ID.
	for _, value := range leases {
		byID[value.ID] = value
	}
	for _, value := range byID {
		putLease(byID, byIP, byScopeIP, byMAC, value)
	}
	i.mu.Lock()
	i.byID, i.byIP, i.byScopeIP, i.byMAC, i.pools = byID, byIP, byScopeIP, byMAC, poolMap
	i.mu.Unlock()
}

// ReplaceChecked validates and atomically publishes a complete snapshot. On
// validation failure the current index remains unchanged so startup recovery
// can keep the data plane unready rather than serving from partial state.
func (i *MemoryIndex) ReplaceChecked(leases []Lease, pools []AddressPool) error {
	if i == nil {
		return errors.New("memory index is nil")
	}
	poolMap, err := validatePools(pools)
	if err != nil {
		return err
	}
	byID := make(map[string]Lease, len(leases))
	for _, value := range leases {
		if value.ID == "" {
			return errors.New("memory index snapshot lease ID is empty")
		}
		if _, exists := byID[value.ID]; exists {
			return fmt.Errorf("memory index snapshot contains duplicate lease ID %q", value.ID)
		}
		if value.IPAddress != "" {
			if net.ParseIP(value.IPAddress).To4() == nil {
				return fmt.Errorf("memory index snapshot lease %q has invalid IPv4 address %q", value.ID, value.IPAddress)
			}
			if pool, ok := poolMap[value.ScopeID]; ok {
				start, end, err := ipv4Range(pool.StartIP, pool.EndIP)
				if err != nil {
					return err
				}
				ip := net.ParseIP(value.IPAddress).To4()
				valueN := uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
				if valueN < start || valueN > end {
					return fmt.Errorf("memory index snapshot lease %q IP %q is outside scope %q pool", value.ID, value.IPAddress, value.ScopeID)
				}
			}
		}
		byID[value.ID] = value
	}
	byIP := make(map[string]Lease, len(leases))
	byScopeIP := make(map[scopedIP]Lease, len(leases))
	byMAC := make(map[string]Lease, len(leases))
	for _, value := range byID {
		putLease(byID, byIP, byScopeIP, byMAC, value)
	}
	i.mu.Lock()
	i.byID, i.byIP, i.byScopeIP, i.byMAC, i.pools = byID, byIP, byScopeIP, byMAC, poolMap
	i.mu.Unlock()
	return nil
}

func validatePools(pools []AddressPool) (map[string]AddressPool, error) {
	poolMap := make(map[string]AddressPool, len(pools))
	for _, pool := range pools {
		if pool.ScopeID == "" {
			return nil, errors.New("memory index snapshot pool scope ID is empty")
		}
		if _, exists := poolMap[pool.ScopeID]; exists {
			return nil, fmt.Errorf("memory index snapshot contains duplicate pool scope %q", pool.ScopeID)
		}
		start, end, err := ipv4Range(pool.StartIP, pool.EndIP)
		if err != nil {
			return nil, fmt.Errorf("memory index snapshot pool %q: %w", pool.ScopeID, err)
		}
		for ip, enabled := range pool.Reservations {
			if !enabled {
				continue
			}
			parsed := net.ParseIP(ip).To4()
			if parsed == nil {
				return nil, fmt.Errorf("memory index snapshot pool %q has invalid reservation %q", pool.ScopeID, ip)
			}
			value := uint32(parsed[0])<<24 | uint32(parsed[1])<<16 | uint32(parsed[2])<<8 | uint32(parsed[3])
			if value < start || value > end {
				return nil, fmt.Errorf("memory index snapshot pool %q reservation %q is outside pool", pool.ScopeID, ip)
			}
		}
		poolMap[pool.ScopeID] = clonePool(pool)
	}
	return poolMap, nil
}

// Upsert applies the post-commit lease state to the index.
func (i *MemoryIndex) Upsert(value Lease) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if old, ok := i.byID[value.ID]; ok {
		removeLease(i.byID, i.byIP, i.byScopeIP, i.byMAC, old)
		rebuildIPIndex(i.byID, i.byIP, i.byScopeIP, old.IPAddress)
		rebuildMACIndex(i.byID, i.byMAC, old.MACAddress)
	}
	putLease(i.byID, i.byIP, i.byScopeIP, i.byMAC, value)
}

func (i *MemoryIndex) Remove(id string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if old, ok := i.byID[id]; ok {
		removeLease(i.byID, i.byIP, i.byScopeIP, i.byMAC, old)
		rebuildIPIndex(i.byID, i.byIP, i.byScopeIP, old.IPAddress)
		rebuildMACIndex(i.byID, i.byMAC, old.MACAddress)
	}
}

func (i *MemoryIndex) SetPools(pools []AddressPool) {
	copyPools := make(map[string]AddressPool, len(pools))
	for _, pool := range pools {
		copyPools[pool.ScopeID] = clonePool(pool)
	}
	i.mu.Lock()
	i.pools = copyPools
	i.mu.Unlock()
}

func (i *MemoryIndex) Get(id string) (*Lease, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	value, ok := i.byID[id]
	return cloneLeaseValue(value, ok)
}

func (i *MemoryIndex) ByMAC(mac string) (*Lease, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	value, ok := i.byMAC[mac]
	return cloneLeaseValue(value, ok)
}

func (i *MemoryIndex) HeldByIP(ip string) (*Lease, bool) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	value, ok := i.byIP[ip]
	return cloneLeaseValue(value, ok)
}

// FindAvailableIP returns the first address in the configured pool that is not
// held and is not covered by an enabled reservation. It preserves the SQLite
// manager's deterministic first-gap semantics. The lookup is not a claim: a
// caller that needs an atomic in-memory reservation must use ClaimAvailable.
func (i *MemoryIndex) FindAvailableIP(scopeID string) (string, error) {
	i.mu.RLock()
	pool, ok := i.pools[scopeID]
	held := make(map[string]struct{}, len(i.byIP))
	for key, value := range i.byScopeIP {
		if key.scope == scopeID && isHeld(value.Status) {
			held[value.IPAddress] = struct{}{}
		}
	}
	i.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("scope %s not found in memory index", scopeID)
	}
	return firstAvailableIP(pool, held)
}

// ClaimAvailable atomically selects and marks the first available address as
// offered in this MemoryIndex. It is intentionally an in-memory migration
// contract only: it provides no durable, cross-process, or cross-node
// guarantee, and must not be treated as a replacement for the SQLite primary
// write until the full WAL-backed LeaseStore is production-wired.
func (i *MemoryIndex) ClaimAvailable(scopeID string, value Lease) (Lease, error) {
	i.mu.Lock()
	defer i.mu.Unlock()

	pool, ok := i.pools[scopeID]
	if !ok {
		return Lease{}, fmt.Errorf("scope %s not found in memory index", scopeID)
	}
	if value.ID == "" {
		return Lease{}, errors.New("memory index claim requires a lease ID")
	}
	if _, exists := i.byID[value.ID]; exists {
		return Lease{}, fmt.Errorf("lease %s already exists in memory index", value.ID)
	}
	held := make(map[string]struct{}, len(i.byScopeIP))
	for key, current := range i.byScopeIP {
		if key.scope == scopeID && isHeld(current.Status) {
			held[current.IPAddress] = struct{}{}
		}
	}
	ip, err := firstAvailableIP(pool, held)
	if err != nil {
		return Lease{}, err
	}
	value.ScopeID = scopeID
	value.IPAddress = ip
	value.Status = LeaseStatusOffered
	putLease(i.byID, i.byIP, i.byScopeIP, i.byMAC, value)
	return value, nil
}

func firstAvailableIP(pool AddressPool, held map[string]struct{}) (string, error) {
	start, end, err := ipv4Range(pool.StartIP, pool.EndIP)
	if err != nil {
		return "", err
	}
	for current := start; ; current++ {
		ip := uint32IP(current)
		if _, found := held[ip]; !found && !pool.Reservations[ip] {
			return ip, nil
		}
		if current == end {
			break
		}
	}
	return "", ErrNoAvailableAddress
}

func isHeld(status LeaseStatus) bool {
	return status == LeaseStatusActive || status == LeaseStatusOffered || status == LeaseStatusConflict
}

func putLease(byID map[string]Lease, byIP map[string]Lease, byScopeIP map[scopedIP]Lease, byMAC map[string]Lease, value Lease) {
	byID[value.ID] = value
	if !isHeld(value.Status) {
		return
	}
	if old, ok := byIP[value.IPAddress]; !ok || leaseWins(value, old) {
		byIP[value.IPAddress] = value
	}
	scopeKey := scopedIP{scope: value.ScopeID, ip: value.IPAddress}
	if old, ok := byScopeIP[scopeKey]; !ok || leaseWins(value, old) {
		byScopeIP[scopeKey] = value
	}
	if value.Status == LeaseStatusActive {
		if old, ok := byMAC[value.MACAddress]; !ok || leaseWins(value, old) {
			byMAC[value.MACAddress] = value
		}
	}
}

func removeLease(byID map[string]Lease, byIP map[string]Lease, byScopeIP map[scopedIP]Lease, byMAC map[string]Lease, value Lease) {
	delete(byID, value.ID)
	if current, ok := byIP[value.IPAddress]; ok && current.ID == value.ID {
		delete(byIP, value.IPAddress)
	}
	scopeKey := scopedIP{scope: value.ScopeID, ip: value.IPAddress}
	if current, ok := byScopeIP[scopeKey]; ok && current.ID == value.ID {
		delete(byScopeIP, scopeKey)
	}
	if current, ok := byMAC[value.MACAddress]; ok && current.ID == value.ID {
		delete(byMAC, value.MACAddress)
	}
}

func rebuildIPIndex(byID map[string]Lease, byIP map[string]Lease, byScopeIP map[scopedIP]Lease, ip string) {
	delete(byIP, ip)
	for key := range byScopeIP {
		if key.ip == ip {
			delete(byScopeIP, key)
		}
	}
	for _, value := range byID {
		if value.IPAddress != ip || !isHeld(value.Status) {
			continue
		}
		if old, ok := byIP[ip]; !ok || leaseWins(value, old) {
			byIP[ip] = value
		}
		scopeKey := scopedIP{scope: value.ScopeID, ip: value.IPAddress}
		if old, ok := byScopeIP[scopeKey]; !ok || leaseWins(value, old) {
			byScopeIP[scopeKey] = value
		}
	}
}

func leaseWins(value, old Lease) bool {
	if value.LeaseEnd != old.LeaseEnd {
		return value.LeaseEnd > old.LeaseEnd
	}
	if value.Generation != old.Generation {
		return value.Generation > old.Generation
	}
	return value.ID > old.ID
}

func rebuildMACIndex(byID map[string]Lease, byMAC map[string]Lease, mac string) {
	delete(byMAC, mac)
	for _, value := range byID {
		if value.MACAddress != mac || value.Status != LeaseStatusActive {
			continue
		}
		if old, ok := byMAC[mac]; !ok || leaseWins(value, old) {
			byMAC[mac] = value
		}
	}
}

func cloneLeaseValue(value Lease, ok bool) (*Lease, bool) {
	if !ok {
		return nil, false
	}
	copy := value
	return &copy, true
}

func clonePool(pool AddressPool) AddressPool {
	pool.Reservations = mapsClone(pool.Reservations)
	return pool
}

func mapsClone(input map[string]bool) map[string]bool {
	if input == nil {
		return nil
	}
	output := make(map[string]bool, len(input))
	for key, value := range input {
		output[key] = value
	}
	return output
}

func ipv4Range(startIP, endIP string) (uint32, uint32, error) {
	start := net.ParseIP(startIP).To4()
	end := net.ParseIP(endIP).To4()
	if start == nil || end == nil {
		return 0, 0, fmt.Errorf("memory index requires IPv4 pool bounds")
	}
	startN := uint32(start[0])<<24 | uint32(start[1])<<16 | uint32(start[2])<<8 | uint32(start[3])
	endN := uint32(end[0])<<24 | uint32(end[1])<<16 | uint32(end[2])<<8 | uint32(end[3])
	if startN > endN {
		return 0, 0, fmt.Errorf("memory index pool start is after end")
	}
	return startN, endN, nil
}

func uint32IP(value uint32) string {
	return fmt.Sprintf("%d.%d.%d.%d", value>>24, byte(value>>16), byte(value>>8), byte(value))
}
