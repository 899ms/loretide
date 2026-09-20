package handler

import (
	"github.com/multica-ai/multica/server/internal/events"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
	"net/http"
	"net/http/httptest"
	"testing"
)

type dingTalkAgentPermissionCase struct {
	name       string
	userID     string
	ownsAgent  bool
	denyStatus int
}

func TestPublishDingTalkInstallationCreated(t *testing.T) {
	bus := events.New()
	h := &Handler{Bus: bus}

	const (
		wsID   = "11111111-1111-1111-1111-111111111111"
		instID = "22222222-2222-2222-2222-222222222222"
	)

	var got events.Event
	fired := 0
	bus.Subscribe(protocol.EventDingTalkInstallationCreated, func(e events.Event) {
		got = e
		fired++
	})

	h.publishDingTalkInstallationCreated(db.ChannelInstallation{
		ID:          parseUUID(instID),
		WorkspaceID: parseUUID(wsID),
	}, "user-1")

	if fired != 1 {
		t.Fatalf("expected dingtalk_installation:created published once, got %d", fired)
	}
	if got.WorkspaceID != wsID || got.ActorType != "user" || got.ActorID != "user-1" {
		t.Errorf("event envelope = %+v", got)
	}
	payload, ok := got.Payload.(map[string]any)
	if !ok || payload["id"] != instID {
		t.Errorf("payload = %v, want installation id %s", got.Payload, instID)
	}
}

func TestListDingTalkGroupsDisabledDeploymentReturnsStableEmptyShape(t *testing.T) {
	rec := httptest.NewRecorder()
	(&Handler{}).ListDingTalkGroups(rec, httptest.NewRequest(http.MethodGet, "/api/workspaces/bad/dingtalk/groups", nil))
	if rec.Code != http.StatusOK || rec.Body.String() != "{\"groups\":[],\"group_discovery_supported\":true,\"inactive_group_counts\":{},\"bot_identities\":{}}\n" {
		t.Fatalf("disabled groups response = %d %q", rec.Code, rec.Body.String())
	}
}
