// Package connections stores public connection settings. Credentials never go to disk.
package connections

import (
	"encoding/json"
	"errors"
	"math"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
)

type AI struct {
	Provider                 string   `json:"provider"`
	InputUSDPerMillion       *float64 `json:"input_usd_per_million"`
	CachedInputUSDPerMillion *float64 `json:"cached_input_usd_per_million"`
	OutputUSDPerMillion      *float64 `json:"output_usd_per_million"`
	BaseURL                  string   `json:"base_url"`
	Model                    string   `json:"model"`
	JSONMode                 bool     `json:"json_mode"`
	TokenField               string   `json:"token_field"`
	MaxQuestions             int      `json:"max_questions"`
}

// Discord is one public interviewer integration, not one Discord account per workflow role.
type Discord struct {
	Enabled         bool   `json:"enabled"`
	GuildID         string `json:"guild_id"`
	CategoryID      string `json:"category_id"`
	ReviewerRoleID  string `json:"reviewer_role_id"`
	BotUserID       string `json:"bot_user_id"`
	DisplayName     string `json:"display_name"`
	AutoCreateRooms bool   `json:"auto_create_rooms"`
}
type Binding struct {
	Agent       string `json:"agent"`
	BotName     string `json:"bot_name"`
	GuildID     string `json:"guild_id"`
	ChannelID   string `json:"channel_id"`
	ChannelName string `json:"channel_name"`
	Enabled     bool   `json:"enabled"`
}

const DefaultInterviewMinutes = 75
const DefaultGreeting = "Hello and welcome. I'm Teamrise, your interview assistant."

type Interview struct {
	Minutes  int    `json:"minutes"`
	Greeting string `json:"greeting"`
}

type Public struct {
	Discord   Discord   `json:"discord"`
	AI        AI        `json:"ai"`
	Bots      []Binding `json:"bots"`
	HasKey    bool      `json:"has_key"`
	Revision  int       `json:"revision"`
	Interview Interview `json:"interview"`
}
type Update struct {
	Discord  *Discord  `json:"discord,omitempty"`
	AI       AI        `json:"ai"`
	Bots     []Binding `json:"bots"`
	APIKey   string    `json:"api_key"`
	ClearKey bool      `json:"clear_key"`
}
type Store struct {
	mu     sync.Mutex
	path   string
	public Public
	key    string
}

func New(path string) (*Store, error) {
	s := &Store{path: path, public: Public{AI: AI{BaseURL: "https://api.openai.com/v1", TokenField: "max_tokens", MaxQuestions: 6}, Discord: Discord{DisplayName: "Echo", AutoCreateRooms: true}, Bots: []Binding{}}}
	if path != "" {
		b, err := os.ReadFile(path)
		if err == nil {
			if err = json.Unmarshal(b, &s.public); err != nil {
				return nil, err
			}
		} else if !os.IsNotExist(err) {
			return nil, err
		}
	}
	s.public.HasKey = false
	if s.public.Interview.Minutes == 0 {
		s.public.Interview.Minutes = DefaultInterviewMinutes
	}
	if s.public.Interview.Greeting == "" {
		s.public.Interview.Greeting = DefaultGreeting
	}
	if s.public.AI.Provider == "" {
		s.public.AI.Provider = "compatible"
	}
	// Retain legacy assignments for compatibility, but migrate only the visible interviewer.
	if s.public.Discord.GuildID == "" {
		for _, b := range s.public.Bots {
			if b.Agent == "Interviewer" && b.GuildID != "" {
				s.public.Discord.GuildID = b.GuildID
				s.public.Discord.DisplayName = b.BotName
				break
			}
		}
	}
	return s, nil
}
func (s *Store) Public() Public {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.public
	p.Bots = append([]Binding{}, p.Bots...)
	p.HasKey = s.key != ""
	return p
}
func (s *Store) Credentials() (AI, string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.public.AI, s.key
}

var snowflake = regexp.MustCompile(`^[0-9]{17,20}$`)
var agents = map[string]bool{"Scout": true, "Interviewer": true, "Analyst": true, "Fairness Auditor": true, "Ops": true, "Concierge": true, "Chief": true}

func (s *Store) Save(u Update) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(u)
}

// SaveDiscord changes only the public Discord policy, preserving concurrent AI
// settings and the session-only API key. The bot token belongs to discordbot.
func (s *Store) SaveDiscord(discord Discord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(Update{AI: s.public.AI, Bots: append([]Binding{}, s.public.Bots...), Discord: &discord})
}

func (s *Store) SaveInterview(cfg Interview) error {
	cfg.Greeting = strings.TrimSpace(cfg.Greeting)
	if cfg.Minutes < 1 || cfg.Minutes > 480 || len([]rune(cfg.Greeting)) > 600 {
		return errors.New("use 1–480 minutes and a greeting of up to 600 characters")
	}
	if cfg.Greeting == "" {
		cfg.Greeting = DefaultGreeting
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.public
	p.Interview = cfg
	p.Revision++
	return s.persistLocked(p, s.key)
}

func (s *Store) saveLocked(u Update) error {
	if u.AI.Provider == "" {
		u.AI.Provider = "compatible"
	}
	if u.AI.Provider != "compatible" && u.AI.Provider != "codex" {
		return errors.New("choose an OpenAI-compatible API or local Codex subscription")
	}
	u.AI.BaseURL = strings.TrimRight(strings.TrimSpace(u.AI.BaseURL), "/")
	u.AI.BaseURL = strings.TrimSuffix(u.AI.BaseURL, "/chat/completions")
	parsed, err := url.Parse(u.AI.BaseURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" || (parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1"))) {
		return errors.New("use an HTTPS API base URL, or HTTP on localhost")
	}
	u.AI.Model = strings.TrimSpace(u.AI.Model)
	if len(u.AI.Model) > 200 || u.AI.MaxQuestions < 1 || u.AI.MaxQuestions > 20 || (u.AI.TokenField != "max_tokens" && u.AI.TokenField != "max_completion_tokens") {
		return errors.New("check model, token parameter and question limit (1–20)")
	}
	if len(u.APIKey) > 4096 || strings.ContainsAny(u.APIKey, "\r\n") {
		return errors.New("invalid API key")
	}
	for _, rate := range []*float64{u.AI.InputUSDPerMillion, u.AI.CachedInputUSDPerMillion, u.AI.OutputUSDPerMillion} {
		if rate != nil && (math.IsNaN(*rate) || math.IsInf(*rate, 0) || *rate < 0 || *rate > 1000000) {
			return errors.New("token rates must be nonnegative USD per million tokens")
		}
	}
	discord := s.public.Discord
	if u.Discord != nil {
		discord = *u.Discord
	}
	discord.DisplayName = strings.TrimSpace(discord.DisplayName)
	if discord.DisplayName == "" {
		discord.DisplayName = "Echo"
	}
	if len(discord.DisplayName) > 80 {
		return errors.New("interviewer name is too long")
	}
	for _, id := range []string{discord.GuildID, discord.CategoryID, discord.ReviewerRoleID, discord.BotUserID} {
		if id != "" && !snowflake.MatchString(id) {
			return errors.New("Discord IDs must contain 17–20 digits")
		}
	}
	if discord.Enabled && (discord.GuildID == "" || discord.CategoryID == "" || discord.BotUserID == "" || !discord.AutoCreateRooms) {
		return errors.New("private rooms require a server, category, interviewer bot ID and automatic room creation")
	}
	if discord.ReviewerRoleID != "" && discord.ReviewerRoleID == discord.GuildID {
		return errors.New("the reviewer role cannot be @everyone")
	}
	seen := map[string]bool{}
	for _, b := range u.Bots {
		if !agents[b.Agent] || seen[b.Agent] || len(b.BotName) > 100 || len(b.ChannelName) > 100 {
			return errors.New("invalid or duplicate bot binding")
		}
		seen[b.Agent] = true
		if (b.GuildID != "" && !snowflake.MatchString(b.GuildID)) || (b.ChannelID != "" && !snowflake.MatchString(b.ChannelID)) || (b.Enabled && (b.GuildID == "" || b.ChannelID == "")) {
			return errors.New("Discord server and channel IDs must contain 17–20 digits")
		}
	}
	key := s.key
	if u.ClearKey {
		key = ""
	}
	if u.APIKey != "" {
		key = u.APIKey
	} else if u.AI.BaseURL != s.public.AI.BaseURL {
		key = ""
	}
	p := Public{AI: u.AI, Discord: discord, Bots: append([]Binding{}, u.Bots...), Revision: s.public.Revision + 1, Interview: s.public.Interview}
	return s.persistLocked(p, key)
}

func (s *Store) persistLocked(p Public, key string) error {
	if s.path != "" {
		if err := os.MkdirAll(filepath.Dir(s.path), 0700); err != nil {
			return err
		}
		b, _ := json.MarshalIndent(p, "", "  ")
		if err := os.WriteFile(s.path+".tmp", b, 0600); err != nil {
			return err
		}
		if err := os.Rename(s.path+".tmp", s.path); err != nil {
			return err
		}
	}
	s.public = p
	s.key = key
	return nil
}
