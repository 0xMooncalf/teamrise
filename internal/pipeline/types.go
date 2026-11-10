package pipeline

import "time"

type Message struct {
	Image           string `json:"image,omitempty"`
	Channel         string `json:"channel,omitempty"`
	Speaker         string `json:"speaker"`
	Text            string `json:"text"`
	ResponseSeconds int    `json:"response_seconds"`
}
type Score struct {
	Label     string `json:"label"`
	Value     int    `json:"value"`
	Evidence  string `json:"evidence"`
	NotScored bool   `json:"not_scored,omitempty"`
}
type InterviewProfile struct {
	Cards       []InterviewCard `json:"cards,omitempty"`
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Track       string          `json:"track"`
	SkillPrompt string          `json:"skill_prompt"`
	Questions   []string        `json:"questions"`
	Criteria    []string        `json:"criteria"`
}
type InterviewCard struct {
	ID      string `json:"id"`
	Channel string `json:"channel"`
	Prompt  string `json:"prompt"`
	Image   string `json:"image,omitempty"`
	Notes   string `json:"notes,omitempty"`
}
type Settings struct {
	Concurrent   int `json:"concurrent"`
	HandoffScore int `json:"handoff_score"`
}
type AgentRun struct {
	Agent   string `json:"agent"`
	Status  string `json:"status"`
	Ticks   int    `json:"ticks"`
	Summary string `json:"summary"`
}
type Participant struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Kind string `json:"kind"`
}
type Room struct {
	ID                string        `json:"id"`
	Name              string        `json:"name"`
	BotPresent        bool          `json:"bot_present"`
	Participants      []Participant `json:"participants"`
	EmptySince        *int          `json:"empty_since"`
	ReconnectDeadline *int          `json:"reconnect_deadline"`
	Expired           bool          `json:"expired"`
}
type Assessment struct {
	Summary                string   `json:"summary"`
	Characterization       string   `json:"characterization"`
	Recommendation         string   `json:"recommendation"`
	Strengths              []string `json:"strengths"`
	Followups              []string `json:"followups"`
	AverageResponseSeconds int      `json:"average_response_seconds"`
}
type Candidate struct {
	Mode             string           `json:"mode,omitempty"`
	Live             *LiveSession     `json:"live,omitempty"`
	ID               string           `json:"id"`
	Name             string           `json:"name"`
	AvatarURL        string           `json:"avatar_url,omitempty"`
	Initials         string           `json:"initials"`
	Job              string           `json:"job"`
	Source           string           `json:"source"`
	Stage            string           `json:"stage"`
	Role             string           `json:"role"`
	Phase            int              `json:"phase"`
	Score            int              `json:"score"`
	Experience       int              `json:"experience"`
	Color            string           `json:"color"`
	NoShow           bool             `json:"no_show"`
	Rescheduled      bool             `json:"rescheduled"`
	Flag             string           `json:"flag"`
	ReviewResolved   bool             `json:"review_resolved"`
	Transcript       []Message        `json:"transcript"`
	Scorecard        []Score          `json:"scorecard"`
	Feedback         []string         `json:"feedback"`
	ProfileID        string           `json:"profile_id"`
	Profile          InterviewProfile `json:"profile"`
	Runs             []AgentRun       `json:"runs"`
	Room             Room             `json:"room"`
	InterviewSeconds int              `json:"interview_seconds"`
	Assessment       *Assessment      `json:"assessment"`
}
type AdminRequest struct {
	RequestedByCandidate bool       `json:"requested_by_candidate,omitempty"`
	CreatedTimestamp     *time.Time `json:"created_timestamp,omitempty"`
	ID                   string     `json:"id"`
	CandidateID          string     `json:"candidate_id"`
	CandidateName        string     `json:"candidate_name"`
	RoomID               string     `json:"room_id"`
	Reason               string     `json:"reason"`
	Status               string     `json:"status"`
	CreatedAt            int        `json:"created_at"`
	OpenedBy             []string   `json:"opened_by"`
	AttendedBy           string     `json:"attended_by"`
	AttendedAt           *int       `json:"attended_at"`
}
type Agent struct {
	Name  string `json:"name"`
	Role  string `json:"role"`
	Color string `json:"color"`
}
type Event struct {
	ID            int       `json:"id"`
	Time          time.Time `json:"time"`
	Agent         string    `json:"agent"`
	CandidateID   string    `json:"candidate_id,omitempty"`
	CandidateName string    `json:"candidate_name,omitempty"`
	Kind          string    `json:"kind"`
	Text          string    `json:"text"`
	Channel       string    `json:"channel"`
}
type Stats struct {
	InterviewSeconds    int `json:"interview_seconds"`
	CompletedInterviews int `json:"completed_interviews"`
	ActiveInterviews    int `json:"active_interviews"`
	LiveRooms           int `json:"live_rooms"`
	PendingRequests     int `json:"pending_requests"`
}
type State struct {
	Demo         bool               `json:"demo"`
	Running      bool               `json:"running"`
	Revision     int                `json:"revision"`
	Candidates   []Candidate        `json:"candidates"`
	Events       []Event            `json:"events"`
	Agents       []Agent            `json:"agents"`
	Profiles     []InterviewProfile `json:"profiles"`
	Settings     Settings           `json:"settings"`
	Requests     []AdminRequest     `json:"requests"`
	ClockSeconds int                `json:"clock_seconds"`
	Stats        Stats              `json:"stats"`
}

var Agents = []Agent{
	{"Scout", "Match the role", "#a78bfa"},
	{"Interviewer", "Have the conversation", "#60a5fa"},
	{"Analyst", "Build an evidence-based summary", "#fbbf24"},
	{"Fairness Auditor", "Check evidence and consistency", "#e879c9"},
	{"Ops", "Keep the room connected", "#34d399"},
	{"Concierge", "Prepare the human handoff", "#fb923c"},
	{"Chief", "Call a human for the final decision", "#a5b4fc"},
}
