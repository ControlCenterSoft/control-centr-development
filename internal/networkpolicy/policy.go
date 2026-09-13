package networkpolicy

import (
	"errors"
	"fmt"
)

var (
	ErrInvalidZone              = errors.New("invalid network zone")
	ErrForwardingDisabled       = errors.New("inter-zone forwarding disabled")
	ErrEdgeGatewayRequired      = errors.New("edge gateway required")
	ErrSupportZoneTransitDenied = errors.New("support zones cannot authorize transit forwarding")
)

// Zone is a first-class network security zone used by Control Center network
// planning. Classifying an interface into a zone never enables forwarding,
// NAT, firewall rules, or any host mutation by itself.
type Zone string

const (
	ZoneWAN        Zone = "WAN"
	ZoneLAN        Zone = "LAN"
	ZoneManagement Zone = "MANAGEMENT"
	ZoneDMZ        Zone = "DMZ"
	ZoneCluster    Zone = "CLUSTER"
	ZoneStorage    Zone = "STORAGE"
	ZoneBackup     Zone = "BACKUP"
	ZoneSupportLAN Zone = "SUPPORT_LAN"
	ZoneSupportWAN Zone = "SUPPORT_WAN"
)

func (z Zone) Valid() bool {
	switch z {
	case ZoneWAN, ZoneLAN, ZoneManagement, ZoneDMZ, ZoneCluster, ZoneStorage, ZoneBackup, ZoneSupportLAN, ZoneSupportWAN:
		return true
	default:
		return false
	}
}

func (z Zone) isSupport() bool {
	return z == ZoneSupportLAN || z == ZoneSupportWAN
}

// ForwardingIntent captures the minimum authorization inputs for routing
// between network zones. Inter-zone routing is fail-closed and WAN forwarding
// additionally requires an explicitly assigned Edge Gateway role.
type ForwardingIntent struct {
	Source              Zone `json:"source"`
	Destination         Zone `json:"destination"`
	ExplicitlyEnabled   bool `json:"explicitly_enabled"`
	EdgeGatewayAssigned bool `json:"edge_gateway_assigned"`
}

// AuthorizeForwarding returns nil only when the requested forwarding is
// permitted by the network safety baseline. Same-zone traffic is not
// considered routed forwarding and therefore does not require an Edge Gateway.
// SUPPORT_LAN/SUPPORT_WAN are deliberately non-transit zones in 0.34: their
// presence models the two independent Support Gateway channels but does not
// create a generic route, NAT, or forwarding authority between either support
// channel and any other zone.
func AuthorizeForwarding(intent ForwardingIntent) error {
	if !intent.Source.Valid() {
		return fmt.Errorf("%w: source %q", ErrInvalidZone, intent.Source)
	}
	if !intent.Destination.Valid() {
		return fmt.Errorf("%w: destination %q", ErrInvalidZone, intent.Destination)
	}
	if intent.Source == intent.Destination {
		return nil
	}
	if intent.Source.isSupport() || intent.Destination.isSupport() {
		return ErrSupportZoneTransitDenied
	}
	if !intent.ExplicitlyEnabled {
		return ErrForwardingDisabled
	}
	if (intent.Source == ZoneWAN || intent.Destination == ZoneWAN) && !intent.EdgeGatewayAssigned {
		return ErrEdgeGatewayRequired
	}
	return nil
}
