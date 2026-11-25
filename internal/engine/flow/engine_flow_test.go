package flow

import (
	"encoding/json"
	"testing"

	"web2api/internal/config"
)

func TestParseVideoOperationID(t *testing.T) {
	payload := json.RawMessage(`[["op-123","project-1","media-1"],["op-456"]]`)
	if got := parseVideoOperationID(payload); got != "op-123" {
		t.Fatalf("parseVideoOperationID() = %q, want op-123", got)
	}
}

func TestParseVideoStatus(t *testing.T) {
	tests := []struct {
		name      string
		payload   string
		wantDone  bool
		wantMedia string
		wantErr   bool
	}{
		{
			name:     "in progress",
			payload:  `[["op-1","project-1","IN_PROGRESS"]]`,
			wantDone: false,
		},
		{
			name:      "completed",
			payload:   `[["op-1","project-1","DONE","media-1/"]]`,
			wantDone:  true,
			wantMedia: "media-1",
		},
		{
			name:      "upstream successful status without explicit media id",
			payload:   `[["op-1","project-1","MEDIA_GENERATION_STATUS_SUCCESSFUL"]]`,
			wantDone:  true,
			wantMedia: "op-1",
		},
		{
			name:      "completed with nested video url and no media id",
			payload:   `[["op-1","project-1","MEDIA_GENERATION_STATUS_SUCCESSFUL",null,{"metadata":{"video":{"fifeUrl":"https://flow-content.google.com/video/media-2/file"}}}]]`,
			wantDone:  true,
			wantMedia: "media-2",
		},
		{
			name:     "failed",
			payload:  `[["op-1","project-1","FAILED"]]`,
			wantDone: true,
		},
		{
			name:    "missing operation",
			payload: `[["op-2","project-1","DONE","media-1"]]`,
			wantErr: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			status, err := parseVideoStatus(json.RawMessage(test.payload), "op-1")
			if test.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseVideoStatus() error = %v", err)
			}
			if status.Done != test.wantDone || status.MediaID != test.wantMedia {
				t.Fatalf("status = %+v, want done=%v media=%q", status, test.wantDone, test.wantMedia)
			}
		})
	}
}

func TestAddAccountReady(t *testing.T) {
	engine := New(config.FlowConfig{}, nil)
	creds := Credentials{Cookies: "OSID=session; SID=account"}
	if err := engine.AddAccount("1", "主号", creds); err != nil {
		t.Fatalf("AddAccount() error = %v", err)
	}
	states := engine.AccountStates()
	if len(states) != 1 {
		t.Fatalf("AccountStates() = %d 条, want 1", len(states))
	}
	if !states[0].Ready || states[0].Status != "ok" {
		t.Fatalf("新账号状态 = ready %v status %q, want ready 且 ok", states[0].Ready, states[0].Status)
	}
	if !engine.Ready() {
		t.Fatal("有可用账号时引擎未就绪")
	}

	if err := engine.ReplaceCredentials("1", Credentials{Cookies: "__Secure-OSID=session; HSID=account", ProjectID: "project-1"}); err != nil {
		t.Fatalf("ReplaceCredentials() error = %v", err)
	}
	states = engine.AccountStates()
	if !states[0].Ready || states[0].Status != "ok" {
		t.Fatalf("更新凭据后状态 = ready %v status %q, want ready 且 ok", states[0].Ready, states[0].Status)
	}
}
