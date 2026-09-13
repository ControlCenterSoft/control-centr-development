package ui

import (
	"context"
	"errors"
	"fmt"
	"time"

	"control-center/internal/incidents"
)

// IncidentOperationalReportProvider builds the ordinary resource/health report
// from the canonical incident read repository. It deliberately has no actor or
// permission selector: authorization is bound by the server route before this
// provider is called.
type IncidentOperationalReportProvider struct {
	reader incidents.Reader
	now    func() time.Time
}

// NewIncidentOperationalReportProvider creates a read-only report provider.
// The clock is injected so freshness/release tests can bind the report to an
// explicit trusted instant without introducing global mutable time state.
func NewIncidentOperationalReportProvider(reader incidents.Reader, now func() time.Time) (*IncidentOperationalReportProvider, error) {
	if reader == nil {
		return nil, errors.New("incident report reader is required")
	}
	if now == nil {
		return nil, errors.New("incident report clock is required")
	}
	return &IncidentOperationalReportProvider{reader: reader, now: now}, nil
}

// OperationalReport loads the complete bounded active-incident set before
// adapting it to ui.operational-report/v1. Resolved incidents remain available
// through the Incident browser/Audit history but are not accumulated forever in
// the current resource-health projection; absence of active incidents still
// yields unknown rather than inventing Healthy.
//
// A truncated, inconsistent or non-progressing pagination boundary fails closed
// instead of being presented as a complete health picture. MaxReportItems is
// applied to affected-resource evidence, not merely incident count, because one
// incident may reference many resources.
func (p *IncidentOperationalReportProvider) OperationalReport(ctx context.Context) (OperationalReport, error) {
	if p == nil || p.reader == nil || p.now == nil {
		return OperationalReport{}, errors.New("incident report provider is unavailable")
	}
	now := p.now().UTC()
	if now.IsZero() {
		return OperationalReport{}, errors.New("incident report trusted time is required")
	}

	query, err := incidents.NormalizeListQuery(incidents.ListQuery{
		Limit:    incidents.MaxListLimit,
		Statuses: []incidents.Status{incidents.StatusOpen, incidents.StatusAcknowledged},
	})
	if err != nil {
		return OperationalReport{}, fmt.Errorf("initialize incident report query: %w", err)
	}
	items := make([]incidents.Incident, 0, incidents.MaxListLimit)
	seen := make(map[string]struct{})
	evidenceCount := 0

	for pageNumber := 0; pageNumber <= MaxReportItems; pageNumber++ {
		page, err := p.reader.List(ctx, query)
		if err != nil {
			return OperationalReport{}, fmt.Errorf("read incident report source: %w", err)
		}
		if len(page.Items) > query.Limit {
			return OperationalReport{}, fmt.Errorf("incident report page exceeds requested limit")
		}

		for index := range page.Items {
			incident := page.Items[index]
			if err := incident.Validate(); err != nil {
				return OperationalReport{}, fmt.Errorf("incident report source contains invalid item: %w", err)
			}
			if incident.Status == incidents.StatusResolved {
				return OperationalReport{}, fmt.Errorf("incident report source violated active-status filter for %q", incident.ObjectID)
			}
			if _, exists := seen[incident.ObjectID]; exists {
				return OperationalReport{}, fmt.Errorf("incident report source repeated object %q", incident.ObjectID)
			}
			seen[incident.ObjectID] = struct{}{}
			evidenceCount += len(incident.AffectedResources)
			if evidenceCount > MaxReportItems {
				return OperationalReport{}, fmt.Errorf("incident report evidence exceeds %d items", MaxReportItems)
			}
			items = append(items, incident)
		}

		if !page.HasMore {
			if page.Next != nil {
				return OperationalReport{}, errors.New("incident report source returned terminal page with next cursor")
			}
			return BuildIncidentOperationalReport(now, incidents.ListPage{Items: items})
		}

		if page.Next == nil || len(page.Items) == 0 {
			return OperationalReport{}, errors.New("incident report source returned incomplete pagination evidence")
		}
		last := page.Items[len(page.Items)-1]
		if !page.Next.StartedAt.Equal(last.StartedAt) || page.Next.ObjectID != last.ObjectID {
			return OperationalReport{}, errors.New("incident report next cursor does not bind the last returned item")
		}
		if query.Before != nil && page.Next.StartedAt.Equal(query.Before.StartedAt) && page.Next.ObjectID == query.Before.ObjectID {
			return OperationalReport{}, errors.New("incident report pagination did not advance")
		}
		nextQuery, err := incidents.NormalizeListQuery(incidents.ListQuery{
			Limit:    incidents.MaxListLimit,
			Statuses: append([]incidents.Status(nil), query.Statuses...),
			Before:   page.Next,
		})
		if err != nil {
			return OperationalReport{}, fmt.Errorf("incident report source returned invalid cursor: %w", err)
		}
		query = nextQuery
	}

	return OperationalReport{}, errors.New("incident report pagination exceeded bounded page count")
}
