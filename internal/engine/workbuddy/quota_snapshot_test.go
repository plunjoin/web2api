package workbuddy

import (
	"testing"
	"time"
)

func TestQuotaSnapshotPreservesWindowsAndFreshness(t *testing.T) {
	now := time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC)
	q := &Quota{ReadAt: now, Windows: []Window{
		{Name: "Credits", Unit: "credits", Limit: 500, Amount: 500, Aside: true},
		{Name: "Extra credits", Unit: "credits", Limit: 1700, Amount: 100, Aside: true, RunsOutAt: now.Add(time.Hour).Format(time.RFC3339), RunsOut: 300},
		{Name: "Total credits", Unit: "credits", Limit: 2200, Amount: 600},
	}}
	s := q.Snapshot(now)
	if s.Stale || s.ReadAt != now.Unix() || len(s.Windows) != 3 {
		t.Fatalf("invalid snapshot: %+v", s)
	}
	if s.Windows[2].Remaining != 1600 || s.Windows[2].Aside {
		t.Fatalf("total: %+v", s.Windows[2])
	}
	if !s.Windows[0].Aside || s.Windows[0].Remaining != 0 {
		t.Fatalf("parts: %+v", s.Windows[0])
	}
	if s.Windows[1].ExpiresAt == "" || s.Windows[1].Expiring != 300 {
		t.Fatalf("expiry: %+v", s.Windows[1])
	}
	if !q.Snapshot(now.Add(16 * time.Minute)).Stale {
		t.Fatal("old snapshot must be marked stale")
	}
	s.Windows[0].Remaining = 42
	if q.Windows[0].Amount != 500 {
		t.Fatal("snapshot must not mutate source quota")
	}
}

func TestAccountStatesExposeServicePlanAndCooldown(t *testing.T) {
	acc := &workbuddyAccount{
		id: "1", label: "intl", enabled: true, ready: true,
		creds:         Credentials{Token: "test", Site: SiteIntl.ID},
		quota:         &Quota{Plan: "Pro", ReadAt: time.Now(), Windows: []Window{{Name: "Total credits", Unit: "credits", Limit: 100, Amount: 20}}},
		cooldownUntil: time.Now().Add(time.Minute), cooldownReason: "rate limited",
	}
	e := &Engine{accounts: []*workbuddyAccount{acc}}
	states := e.AccountStates()
	if len(states) != 1 || states[0].Service != SiteIntl.ID || states[0].Tier != "Pro" {
		t.Fatalf("missing service or plan: %+v", states)
	}
	if states[0].Status != "cooldown" || states[0].Ready || len(states[0].Cooldowns) != 1 {
		t.Fatalf("cooldown must not be an authentication error: %+v", states[0])
	}
	if states[0].QuotaSnapshot == nil || states[0].QuotaSnapshot.Windows[0].Remaining != 80 {
		t.Fatalf("missing structured quota: %+v", states[0])
	}
}

func TestQuotaSnapshotInvalidatesAtWindowBoundary(t *testing.T) {
	now := time.Date(2026, 11, 9, 12, 0, 0, 0, time.UTC)
	for _, field := range []string{"reset", "expiry"} {
		for _, tc := range []struct {
			name     string
			boundary string
			stale    bool
		}{
			{"past", now.Add(-time.Second).Format(time.RFC3339), true},
			{"exact", now.Format(time.RFC3339), true},
			{"future", now.Add(time.Second).Format(time.RFC3339), false},
			{"invalid", "invalid", true},
			{"unknown", "", false},
		} {
			t.Run(field+"/"+tc.name, func(t *testing.T) {
				part := Window{Name: "Extra credits", Limit: 100, Amount: 25, Aside: true}
				if field == "reset" {
					part.ResetsAt = tc.boundary
				} else {
					part.RunsOutAt = tc.boundary
				}
				q := &Quota{ReadAt: now, Windows: []Window{part, {Name: "Total credits", Limit: 200, Amount: 50}}}
				s := q.Snapshot(now)
				if s.Stale != tc.stale || s.Windows[1].Remaining != 150 {
					t.Fatalf("stale=%v, want %v; history must remain intact: %+v", s.Stale, tc.stale, s)
				}
				if q.Windows[1].Amount != 50 || q.Exhausted() {
					t.Fatal("presentation must not change routing quota")
				}
			})
		}
	}
}

func TestQuotaSnapshotUnknownAndOverdrawn(t *testing.T) {
	var absent *Quota
	if absent.Snapshot(time.Now()) != nil {
		t.Fatal("unknown is not zero")
	}
	failed := (&Quota{Err: "temporarily unavailable"}).Snapshot(time.Now())
	if failed.Error == "" || !failed.Stale || failed.ReadAt != 0 {
		t.Fatalf("error lost: %+v", failed)
	}
	q := &Quota{ReadAt: time.Now(), Windows: []Window{{Name: "Total credits", Limit: 1, Amount: 2}}}
	if q.Snapshot(time.Now()).Windows[0].Remaining != 0 {
		t.Fatal("negative remaining")
	}
}
