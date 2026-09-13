package networkpolicy

import (
	"errors"
	"testing"
)

func TestAuthorizeForwardingFailsClosedAcrossWAN(t *testing.T) {
	tests := []struct {
		name   string
		intent ForwardingIntent
		want   error
	}{
		{
			name:   "not explicitly enabled",
			intent: ForwardingIntent{Source: ZoneWAN, Destination: ZoneLAN, EdgeGatewayAssigned: true},
			want:   ErrForwardingDisabled,
		},
		{
			name:   "edge gateway not assigned",
			intent: ForwardingIntent{Source: ZoneWAN, Destination: ZoneLAN, ExplicitlyEnabled: true},
			want:   ErrEdgeGatewayRequired,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := AuthorizeForwarding(tc.intent); !errors.Is(err, tc.want) {
				t.Fatalf("AuthorizeForwarding() error = %v, want %v", err, tc.want)
			}
		})
	}
}

func TestAuthorizeForwardingAllowsExplicitEdgeGatewayRouting(t *testing.T) {
	intent := ForwardingIntent{
		Source:              ZoneWAN,
		Destination:         ZoneLAN,
		ExplicitlyEnabled:   true,
		EdgeGatewayAssigned: true,
	}
	if err := AuthorizeForwarding(intent); err != nil {
		t.Fatalf("AuthorizeForwarding() error = %v", err)
	}
}

func TestAuthorizeForwardingAllowsExplicitInternalInterZoneRouting(t *testing.T) {
	intent := ForwardingIntent{
		Source:            ZoneLAN,
		Destination:       ZoneManagement,
		ExplicitlyEnabled: true,
	}
	if err := AuthorizeForwarding(intent); err != nil {
		t.Fatalf("AuthorizeForwarding() error = %v", err)
	}
}

func TestAuthorizeForwardingRejectsSupportZonesAsTransit(t *testing.T) {
	tests := []ForwardingIntent{
		{Source: ZoneSupportLAN, Destination: ZoneManagement, ExplicitlyEnabled: true, EdgeGatewayAssigned: true},
		{Source: ZoneManagement, Destination: ZoneSupportLAN, ExplicitlyEnabled: true, EdgeGatewayAssigned: true},
		{Source: ZoneSupportWAN, Destination: ZoneWAN, ExplicitlyEnabled: true, EdgeGatewayAssigned: true},
		{Source: ZoneWAN, Destination: ZoneSupportWAN, ExplicitlyEnabled: true, EdgeGatewayAssigned: true},
		{Source: ZoneSupportLAN, Destination: ZoneSupportWAN, ExplicitlyEnabled: true, EdgeGatewayAssigned: true},
	}
	for _, intent := range tests {
		if err := AuthorizeForwarding(intent); !errors.Is(err, ErrSupportZoneTransitDenied) {
			t.Fatalf("AuthorizeForwarding(%s -> %s) error = %v, want ErrSupportZoneTransitDenied", intent.Source, intent.Destination, err)
		}
	}
}

func TestSupportZonesAreValidClassifications(t *testing.T) {
	for _, zone := range []Zone{ZoneSupportLAN, ZoneSupportWAN} {
		if !zone.Valid() {
			t.Fatalf("support zone %q is not valid", zone)
		}
	}
	if err := AuthorizeForwarding(ForwardingIntent{Source: ZoneSupportLAN, Destination: ZoneSupportLAN}); err != nil {
		t.Fatalf("same-zone support traffic should not be treated as routed forwarding: %v", err)
	}
}

func TestAuthorizeForwardingRejectsUnknownZone(t *testing.T) {
	err := AuthorizeForwarding(ForwardingIntent{Source: Zone("UNKNOWN"), Destination: ZoneLAN})
	if !errors.Is(err, ErrInvalidZone) {
		t.Fatalf("AuthorizeForwarding() error = %v, want ErrInvalidZone", err)
	}
}
