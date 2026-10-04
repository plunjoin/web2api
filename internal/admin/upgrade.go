package admin

import (
	"errors"
	"net/http"

	"web2api/internal/upgrade"
)

func (a *API) handleUpgradeStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, a.updater.Status(r.Context()))
}

func (a *API) handleUpgradeCheck(w http.ResponseWriter, r *http.Request) {
	a.upgradeResult(w, a.updater.Check())
}

func (a *API) handleUpgradeStart(w http.ResponseWriter, r *http.Request) {
	a.upgradeResult(w, a.updater.Start(r.Context()))
}

func (a *API) upgradeResult(w http.ResponseWriter, err error) {
	if err != nil {
		status := http.StatusServiceUnavailable
		if errors.Is(err, upgrade.ErrConflict) {
			status = http.StatusConflict
		}
		writeJSON(w, status, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{"accepted": true})
}
