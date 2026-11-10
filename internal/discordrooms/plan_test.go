package discordrooms

import (
	"ladder/internal/connections"
	"strconv"
	"testing"
)

func TestPrivateRoomAllowsOnlyIntendedPrincipals(t *testing.T) {
	cfg := connections.Discord{Enabled: true, AutoCreateRooms: true, GuildID: "11111111111111111", CategoryID: "22222222222222222", BotUserID: "33333333333333333", ReviewerRoleID: "44444444444444444"}
	member := "55555555555555555"
	plan, err := Build(cfg, member)
	if err != nil {
		t.Fatal(err)
	}
	if plan.ChannelID != "" || plan.Status != "planned" || plan.Payload.Type != 2 || plan.Payload.ParentID != cfg.CategoryID {
		t.Fatal("plan pretends a Discord channel exists")
	}
	const view = uint64(1 << 10)
	const connect = uint64(1 << 20)
	allowed := map[string]bool{member: true, cfg.BotUserID: true, cfg.ReviewerRoleID: true}
	for _, o := range plan.Payload.PermissionOverwrites {
		allow, _ := strconv.ParseUint(o.Allow, 10, 64)
		deny, _ := strconv.ParseUint(o.Deny, 10, 64)
		if o.ID == cfg.GuildID {
			if o.Type != 0 || allow != 0 || deny&view == 0 || deny&connect == 0 {
				t.Fatal("@everyone can see the room")
			}
		} else {
			if !allowed[o.ID] || allow&view == 0 || allow&connect == 0 {
				t.Fatal("unexpected identity or missing room access")
			}
			delete(allowed, o.ID)
		}
	}
	if len(allowed) != 0 {
		t.Fatal("participant access rule missing")
	}
	cfg.ReviewerRoleID = cfg.GuildID
	if _, err = Build(cfg, member); err == nil {
		t.Fatal("@everyone accepted as reviewer role")
	}
	cfg.ReviewerRoleID = ""
	if _, err = Build(cfg, "bad"); err == nil {
		t.Fatal("invalid participant ID accepted")
	}
	cfg.Enabled = false
	if _, err = Build(cfg, member); err == nil {
		t.Fatal("disabled policy accepted")
	}
}
