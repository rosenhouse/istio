package bosh

import (
	"fmt"
	"net"
	"sync"
)

type VirtualIP string

type VIPStore interface {
	// Get returns a VirtualIP for the given hostname
	// If this is the first time that Get has been called with the hostname,
	// the VIP will be reserved.  This ensures that Get will
	// will return the same result for all subsequent calls with that hostname
	Get(hostname Hostname) (VirtualIP, error)
}

// TODO: share across Pilot instances and persist
// since the lifecycle should match that of the deployment
func NewVIPStore() VIPStore {
	return &vipStore{
		reservations: make(map[Hostname]VirtualIP),
		cursor:       0,
		baseAddress:  net.IP{169, 254, 255, 0},
	}
}

type vipStore struct {
	reservations map[Hostname]VirtualIP

	mu          sync.Mutex
	cursor      byte
	baseAddress net.IP
}

func (s *vipStore) Get(hostname Hostname) (VirtualIP, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if vip, exists := s.reservations[hostname]; exists {
		return vip, nil
	}

	// TODO: addition with carry
	s.cursor++ // start at 1
	const vipCapacity = 254
	if s.cursor > vipCapacity {
		return "", fmt.Errorf("used up all %d VIPs!", vipCapacity)
	}

	ba := s.baseAddress
	newIP := net.IP{
		ba[0],
		ba[1],
		ba[2],
		ba[3] + s.cursor,
	}
	newVIP := VirtualIP(newIP.String())
	s.reservations[hostname] = newVIP
	return newVIP, nil
}
