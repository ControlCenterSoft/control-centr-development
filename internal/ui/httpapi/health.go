package httpapi

import (
	"encoding/json"
	"net/http"

	productui "control-center/internal/ui"
)

// HealthOverviewHandler exposes only a canonical read-only Health projection.
// Authentication/RBAC are deliberately bound by the caller. Provider errors,
// malformed output and unavailable source evidence fail closed as an explicit
// unavailable/unknown Health view rather than a successful empty response.
func HealthOverviewHandler(provider productui.HealthOverviewProvider) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeHealthJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method_not_allowed"})
			return
		}
		if provider == nil {
			writeHealthUnavailable(w)
			return
		}

		view, err := provider.HealthOverview(r.Context())
		if err != nil || productui.ValidateHealthOverview(view) != nil {
			writeHealthUnavailable(w)
			return
		}
		if view.DataState == productui.HealthDataUnavailable {
			writeHealthJSON(w, http.StatusServiceUnavailable, view)
			return
		}
		writeHealthJSON(w, http.StatusOK, view)
	})
}

func writeHealthUnavailable(w http.ResponseWriter) {
	writeHealthJSON(w, http.StatusServiceUnavailable, productui.HealthOverview{
		Schema:             productui.HealthOverviewSchemaV1,
		DataState:          productui.HealthDataUnavailable,
		OverallState:       productui.HealthStateUnknown,
		Resources:          []productui.ResourceHealthView{},
		Signals:            []productui.HealthSignalView{},
		MutationAuthorized: false,
	})
}

func writeHealthJSON(w http.ResponseWriter, status int, value any) {
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
