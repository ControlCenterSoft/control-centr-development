package ui

import (
	"context"
	"errors"
	"time"
)

// HealthSignalSnapshot is the bounded authoritative input accepted by the
// Health -> Reports bridge. Loaded=false means the source is unavailable and
// must not carry partial signal evidence.
type HealthSignalSnapshot struct {
	Loaded  bool
	Signals []HealthSignal
}

// HealthSignalSource exposes already-bounded provider-neutral Health signals.
// Implementations remain responsible for obtaining authoritative observations;
// this boundary deliberately does not infer health from arbitrary resource
// status strings, provider payloads or transport errors.
type HealthSignalSource interface {
	HealthSignals(context.Context) (HealthSignalSnapshot, error)
}

// HealthOverviewProvider exposes the canonical, read-only Health Overview.
// Authorization remains outside the provider so the server can bind the route
// to a fixed RBAC permission without any client-selected source switching.
type HealthOverviewProvider interface {
	HealthOverview(context.Context) (HealthOverview, error)
}

// HealthOperationalReportProvider converts an authoritative Health signal
// snapshot into the qualified Health Overview and then the common read-only
// Operational Report contract. Source failures become an explicit unavailable
// view/report instead of a synthetic empty/Healthy result.
type HealthOperationalReportProvider struct {
	source       HealthSignalSource
	now          func() time.Time
	staleAfter   time.Duration
	expiredAfter time.Duration
}

var (
	_ OperationalReportProvider = (*HealthOperationalReportProvider)(nil)
	_ HealthOverviewProvider    = (*HealthOperationalReportProvider)(nil)
)

func NewHealthOperationalReportProvider(
	source HealthSignalSource,
	now func() time.Time,
	staleAfter time.Duration,
	expiredAfter time.Duration,
) (*HealthOperationalReportProvider, error) {
	if source == nil {
		return nil, errors.New("health signal source is required")
	}
	if now == nil {
		return nil, errors.New("trusted clock is required")
	}
	if staleAfter <= 0 || expiredAfter <= staleAfter {
		return nil, errors.New("freshness budgets must satisfy 0 < stale_after < expired_after")
	}
	return &HealthOperationalReportProvider{
		source:       source,
		now:          now,
		staleAfter:   staleAfter,
		expiredAfter: expiredAfter,
	}, nil
}

// HealthOverview returns the canonical Health projection for the same exact
// authoritative snapshot consumed by OperationalReport. It never invents
// current evidence after a source failure.
func (p *HealthOperationalReportProvider) HealthOverview(ctx context.Context) (HealthOverview, error) {
	overview, _, err := p.buildHealthOverview(ctx)
	return overview, err
}

func (p *HealthOperationalReportProvider) OperationalReport(ctx context.Context) (OperationalReport, error) {
	overview, now, err := p.buildHealthOverview(ctx)
	if err != nil {
		return OperationalReport{}, err
	}
	return BuildHealthOperationalReport(now, overview)
}

func (p *HealthOperationalReportProvider) buildHealthOverview(ctx context.Context) (HealthOverview, time.Time, error) {
	if p == nil || p.source == nil || p.now == nil {
		return HealthOverview{}, time.Time{}, errors.New("health operational report provider is not configured")
	}
	if err := ctx.Err(); err != nil {
		return HealthOverview{}, time.Time{}, err
	}

	now := p.now().UTC()
	if now.IsZero() {
		return HealthOverview{}, time.Time{}, errors.New("trusted clock returned zero time")
	}

	snapshot, sourceErr := p.source.HealthSignals(ctx)
	if err := ctx.Err(); err != nil {
		return HealthOverview{}, time.Time{}, err
	}
	if sourceErr != nil {
		snapshot = HealthSignalSnapshot{Loaded: false}
	}

	overview, err := BuildHealthOverview(HealthOverviewInput{
		Loaded:       snapshot.Loaded,
		Signals:      snapshot.Signals,
		Now:          now,
		StaleAfter:   p.staleAfter,
		ExpiredAfter: p.expiredAfter,
	})
	if err != nil {
		return HealthOverview{}, time.Time{}, err
	}
	return overview, now, nil
}
