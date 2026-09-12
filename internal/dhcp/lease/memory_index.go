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
	mu    sync.RWMutex
	byID  map[string]Lease
	byIP  map[string]Lease
	byMAC map[string]Lease
	pools map[string]AddressPool
}

func NewMemoryIndex() *MemoryIndex {
	return &MemoryIndex{
		byID:  make(map[string]Lease),
		byIP:  make(map[string]Lease),
		byMAC: make(map[string]Lease),
		pools: make(map[string]AddressPool),
	}
}

// Replace atomically rebuilds all indexes from a durable snapshot.
func (i *MemoryIndex) Replace(leases []Lease, pools []AddressPool) {
	byID := make(map[string]Lease, len(leases))
	byIP := make(map[string]Lease, len(leases))
	byMAC := make(map[string]Lease, len(leases))
	poolMap := make(map[string]AddressPool, len(pools))
	for _, pool := range pools {
		poolMap[pool.ScopeID] = clonePool(pool)
	}
	for _, value := range leases {
		putLease(byID, byIP, byMAC, value)
	}
	i.mu.Lock()
	i.byID, i.byIP, i.byMAC, i.pools = byID, byIP, byMAC, poolMap
	i.mu.Unlock()
}

// Upsert applies the post-commit lease state to the index.
func (i *MemoryIndex) Upsert(value Lease) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if old, ok := i.byID[value.ID]; ok {
		removeLease(i.byID, i.byIP, i.byMAC, old)
	}
	putLease(i.byID, i.byIP, i.byMAC, value)
}

func (i *MemoryIndex) Remove(id string) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if old, ok := i.byID[id]; ok {
		removeLease(i.byID, i.byIP, i.byMAC, old)
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
// manager's deterministic first-gap semantics.
func (i *MemoryIndex) FindAvailableIP(scopeID string) (string, error) {
	i.mu.RLock()
	pool, ok := i.pools[scopeID]
	held := make(map[string]struct{}, len(i.byIP))
	for ip, value := range i.byIP {
		if value.ScopeID == scopeID && isHeld(value.Status) {
			held[ip] = struct{}{}
		}
	}
	i.mu.RUnlock()
	if !ok {
		return "", fmt.Errorf("scope %s not found in memory index", scopeID)
	}
	start, end, err := ipv4Range(pool.StartIP, pool.EndIP)
	if err != nil {
		return "", err
	}
	for current := start; current <= end; current++ {
		ip := uint32IP(current)
		if _, found := held[ip]; found {
			continue
		}
		if pool.Reservations[ip] {
			continue
		}
		return ip, nil
	}
	return "", ErrNoAvailableAddress
}

func isHeld(status LeaseStatus) bool {
	return status == LeaseStatusActive || status == LeaseStatusOffered || status == LeaseStatusConflict
}

func putLease(byID map[string]Lease, byIP map[string]Lease, byMAC map[string]Lease, value Lease) {
	byID[value.ID] = value
	if !isHeld(value.Status) {
		return
	}
	if old, ok := byIP[value.IPAddress]; !ok || value.LeaseEnd > old.LeaseEnd {
		byIP[value.IPAddress] = value
	}
	if value.Status == LeaseStatusActive {
		if old, ok := byMAC[value.MACAddress]; !ok || value.LeaseEnd > old.LeaseEnd {
			byMAC[value.MACAddress] = value
		}
	}
}

func removeLease(byID map[string]Lease, byIP map[string]Lease, byMAC map[string]Lease, value Lease) {
	delete(byID, value.ID)
	if current, ok := byIP[value.IPAddress]; ok && current.ID == value.ID {
		delete(byIP, value.IPAddress)
	}
	if current, ok := byMAC[value.MACAddress]; ok && current.ID == value.ID {
		delete(byMAC, value.MACAddress)
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
