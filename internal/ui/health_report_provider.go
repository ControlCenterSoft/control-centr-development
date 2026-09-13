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

// HealthOperationalReportProvider converts an authoritative Health signal
// snapshot into the qualified Health Overview and then the common read-only
// Operational Report contract. Source failures become an explicit unavailable
// report instead of a synthetic empty/Healthy result.
type HealthOperationalReportProvider struct {
	source       HealthSignalSource
	now          func() time.Time
	staleAfter   time.Duration
	expiredAfter time.Duration
}

var _ OperationalReportProvider = (*HealthOperationalReportProvider)(nil)

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

func (p *HealthOperationalReportProvider) OperationalReport(ctx context.Context) (OperationalReport, error) {
	if p == nil || p.source == nil || p.now == nil {
		return OperationalReport{}, errors.New("health operational report provider is not configured")
	}
	if err := ctx.Err(); err != nil {
		return OperationalReport{}, err
	}

	now := p.now().UTC()
	if now.IsZero() {
		return OperationalReport{}, errors.New("trusted clock returned zero time")
	}

	snapshot, sourceErr := p.source.HealthSignals(ctx)
	if err := ctx.Err(); err != nil {
		return OperationalReport{}, err
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
		return OperationalReport{}, err
	}
	return BuildHealthOperationalReport(now, overview)
}
