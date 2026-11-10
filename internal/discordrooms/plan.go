// Package discordrooms prepares fail-closed private voice-room requests.
// The voice service verifies these permissions before joining an actual room.
package discordrooms

import (
	"errors"
	"ladder/internal/connections"
	"regexp"
	"strconv"
	"strings"
)

type Overwrite struct {
	ID    string `json:"id"`
	Type  int    `json:"type"`
	Allow string `json:"allow"`
	Deny  string `json:"deny"`
}
type Payload struct {
	Name                 string      `json:"name"`
	Type                 int         `json:"type"`
	ParentID             string      `json:"parent_id"`
	PermissionOverwrites []Overwrite `json:"permission_overwrites"`
}
type Plan struct {
	AvatarURL string  `json:"avatar_url,omitempty"`
	GuildID   string  `json:"guild_id"`
	MemberID  string  `json:"member_id"`
	ChannelID string  `json:"channel_id"`
	Status    string  `json:"status"`
	Payload   Payload `json:"payload"`
}

var snowflake = regexp.MustCompile(`^[0-9]{17,20}$`)

func Build(cfg connections.Discord, memberID string) (*Plan, error) {
	if !cfg.Enabled || !cfg.AutoCreateRooms {
		return nil, errors.New("enable the private-room policy in Discord settings first")
	}
	for _, id := range []string{cfg.GuildID, cfg.CategoryID, cfg.BotUserID, memberID} {
		if !snowflake.MatchString(id) {
			return nil, errors.New("a server, category, interviewer bot and candidate Discord member ID are required")
		}
	}
	if memberID == cfg.BotUserID || memberID == cfg.GuildID || cfg.ReviewerRoleID == cfg.GuildID {
		return nil, errors.New("candidate, bot and reviewer identities must be distinct from @everyone")
	}
	if cfg.ReviewerRoleID != "" && !snowflake.MatchString(cfg.ReviewerRoleID) {
		return nil, errors.New("invalid reviewer role")
	}
	const view = uint64(1 << 10)
	const connect = uint64(1 << 20)
	const speak = uint64(1 << 21)
	const send = uint64(1 << 11)
	const history = uint64(1 << 16)
	const attach = uint64(1 << 15)
	bits := view | connect | speak | send | history
	permission := strconv.FormatUint(bits, 10)
	overwrites := []Overwrite{{ID: cfg.GuildID, Type: 0, Allow: "0", Deny: strconv.FormatUint(view|connect, 10)}, {ID: memberID, Type: 1, Allow: permission, Deny: "0"}, {ID: cfg.BotUserID, Type: 1, Allow: strconv.FormatUint(bits|attach, 10), Deny: "0"}}
	if cfg.ReviewerRoleID != "" {
		overwrites = append(overwrites, Overwrite{ID: cfg.ReviewerRoleID, Type: 0, Allow: permission, Deny: "0"})
	}
	// A pseudonymous channel name avoids exposing candidate names in server lists/logs.
	return &Plan{GuildID: cfg.GuildID, MemberID: memberID, Status: "planned", Payload: Payload{Name: "interview-" + strings.TrimPrefix(memberID, "candidate-") + "-private", Type: 2, ParentID: cfg.CategoryID, PermissionOverwrites: overwrites}}, nil
}
