package server

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/kanzifucius/xp-tracker/pkg/store"
)

// ClaimDTO is the JSON representation of a single Crossplane claim.
type ClaimDTO struct {
	Group       string `json:"group"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Creator     string `json:"creator"`
	Team        string `json:"team"`
	Composition string `json:"composition"`
	Ready       bool   `json:"ready"`
	Reason      string `json:"reason"`
	AgeSeconds  int64  `json:"ageSeconds"`
}

// XRDTO is the JSON representation of a single Crossplane composite resource.
type XRDTO struct {
	Group       string `json:"group"`
	Kind        string `json:"kind"`
	Namespace   string `json:"namespace"`
	Name        string `json:"name"`
	Composition string `json:"composition"`
	Ready       bool   `json:"ready"`
	Reason      string `json:"reason"`
	AgeSeconds  int64  `json:"ageSeconds"`
}

// BookkeepingResponse is the top-level JSON response for the /bookkeeping endpoint.
type BookkeepingResponse struct {
	Claims      []ClaimDTO `json:"claims"`
	XRs         []XRDTO    `json:"xrs"`
	GeneratedAt string     `json:"generatedAt"`
}

// bookkeepingHandler returns an http.HandlerFunc that serves the bookkeeping JSON endpoint.
func bookkeepingHandler(s store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		now := time.Now().UTC()

		claims := s.SnapshotClaims()
		xrs := s.SnapshotXRs()

		claimDTOs := make([]ClaimDTO, 0, len(claims))
		for _, c := range claims {
			age := int64(now.Sub(c.CreatedAt).Seconds())
			claimDTOs = append(claimDTOs, ClaimDTO{
				Group:       c.Group,
				Kind:        c.Kind,
				Namespace:   c.Namespace,
				Name:        c.Name,
				Creator:     c.Creator,
				Team:        c.Team,
				Composition: c.Composition,
				Ready:       c.Ready,
				Reason:      c.Reason,
				AgeSeconds:  age,
			})
		}

		xrDTOs := make([]XRDTO, 0, len(xrs))
		for _, x := range xrs {
			age := int64(now.Sub(x.CreatedAt).Seconds())
			xrDTOs = append(xrDTOs, XRDTO{
				Group:       x.Group,
				Kind:        x.Kind,
				Namespace:   x.Namespace,
				Name:        x.Name,
				Composition: x.Composition,
				Ready:       x.Ready,
				Reason:      x.Reason,
				AgeSeconds:  age,
			})
		}

		resp := BookkeepingResponse{
			Claims:      claimDTOs,
			XRs:         xrDTOs,
			GeneratedAt: now.Format(time.RFC3339),
		}

		data, err := json.Marshal(resp)
		if err != nil {
			slog.Error("failed to marshal bookkeeping response", "error", err)
			http.Error(w, "failed to encode response", http.StatusInternalServerError)
			return
		}

		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write(data)
	}
}
