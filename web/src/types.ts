export type Message = {
  speaker: string;
  image?: string;
  channel?: "chat";
  text: string;
  response_seconds: number;
};
export type InterviewProfile = {
  cards?: InterviewCard[];
  id: string;
  name: string;
  track: string;
  skill_prompt: string;
  questions: string[];
  criteria: string[];
};
export type InterviewCard = {
  id: string;
  channel: "voice" | "chat";
  prompt: string;
  image?: string;
  notes?: string;
};
export type AgentRun = {
  agent: string;
  status: "idle" | "active" | "done";
  ticks: number;
  summary: string;
};
export type Room = {
  id: string;
  name: string;
  bot_present: boolean;
  participants: { id: string; name: string; kind: string }[];
  empty_since: number | null;
  reconnect_deadline: number | null;
  expired: boolean;
};
export type Assessment = {
  summary: string;
  characterization: string;
  recommendation: string;
  strengths: string[];
  followups: string[];
  average_response_seconds: number;
};
export type Candidate = {
  mode?: string;
  live?: {
    status: string;
    failure?: string;
    error?: string;
    version: number;
    model: string;
    binding_agent: string;
    started_at: string;
    completed_at: string | null;
    scored: boolean;
    reviewer: string;
    limitations: string[];
    max_questions: number;
    deadline?: string;
    calls?: UsageCall[];
    voice?: {
      guild_id: string;
      member_id: string;
      state: string;
      stopped: boolean;
      error?: string;
      empty_deadline?: string;
      reconnect_grace_seconds?: number;
      calls: SpeechCall[];
    };
    discord?: PrivateRoomPlan;
  };
  id: string;
  name: string;
  initials: string;
  avatar_url?: string;
  job: string;
  source: string;
  stage: string;
  role: string;
  phase: number;
  score: number;
  experience: number;
  color: string;
  no_show: boolean;
  rescheduled: boolean;
  flag: string;
  review_resolved: boolean;
  transcript: Message[];
  scorecard: {
    label: string;
    value: number;
    evidence: string;
    not_scored?: boolean;
  }[];
  feedback: string[];
  profile_id: string;
  profile: InterviewProfile;
  runs: AgentRun[];
  room: Room;
  interview_seconds: number;
  assessment: Assessment | null;
};
export type Agent = { name: string; role: string; color: string };
export type Event = {
  id: number;
  time: string;
  agent: string;
  candidate_id?: string;
  candidate_name?: string;
  kind: string;
  text: string;
  channel: string;
};
export type AdminRequest = {
  created_timestamp?: string;
  id: string;
  candidate_id: string;
  candidate_name: string;
  room_id: string;
  reason: string;
  status: "pending" | "attended" | "closed";
  created_at: number;
  opened_by: string[];
  attended_by: string;
  attended_at: number | null;
};
export type Settings = { concurrent: number; handoff_score: number };
export type State = {
  voice_connection?: VoiceConnection;
  discord_connection: DiscordConnection;
  connections: Connections;
  demo: boolean;
  running: boolean;
  revision: number;
  candidates: Candidate[];
  events: Event[];
  agents: Agent[];
  profiles: InterviewProfile[];
  settings: Settings;
  requests: AdminRequest[];
  clock_seconds: number;
  stats: {
    interview_seconds: number;
    completed_interviews: number;
    active_interviews: number;
    live_rooms: number;
    pending_requests: number;
  };
};
export type VoiceConnection = {
  revision: number;
  enabled: boolean;
  ready: boolean;
  state: string;
  message: string;
  candidate_id?: string;
  channel_id?: string;
  language: string;
};
export type SpeechCall = {
  kind: string;
  model: string;
  audio_seconds: number;
  characters: number;
  duration_ms: number;
  at: string;
};
export type Action = (path: string, body?: object) => Promise<boolean>;
export type AIConfig = {
  provider: "compatible" | "codex";
  input_usd_per_million: number | null;
  cached_input_usd_per_million: number | null;
  output_usd_per_million: number | null;
  base_url: string;
  model: string;
  json_mode: boolean;
  token_field: string;
  max_questions: number;
};
export type BotBinding = {
  agent: string;
  bot_name: string;
  guild_id: string;
  channel_id: string;
  channel_name: string;
  enabled: boolean;
};
export type Connections = {
  interview: InterviewSettings;
  discord: DiscordConfig;
  ai: AIConfig;
  bots: BotBinding[];
  has_key: boolean;
  revision: number;
};
export type InterviewSettings = { minutes: number; greeting: string };
export type DiscordConfig = {
  enabled: boolean;
  guild_id: string;
  category_id: string;
  reviewer_role_id: string;
  bot_user_id: string;
  display_name: string;
  auto_create_rooms: boolean;
};
export type DiscordConnection = {
  revision: number;
  connected: boolean;
  has_token: boolean;
  gateway: string;
  bot_id?: string;
  bot_name?: string;
  guilds: { id: string; name: string }[];
  message: string;
  voice_ready: boolean;
};
export type PrivateRoomPlan = {
  guild_id: string;
  member_id: string;
  channel_id: string;
  status: string;
  payload: {
    name: string;
    parent_id: string;
    type: number;
    permission_overwrites: {
      id: string;
      type: number;
      allow: string;
      deny: string;
    }[];
  };
};
export type UsageCall = {
  id: string;
  agent: string;
  at: string;
  usage: {
    provider?: string;
    billing?: string;
    reported: boolean;
    input_tokens: number;
    output_tokens: number;
    total_tokens: number;
    cached_tokens: number;
    reasoning_tokens: number;
    model: string;
    duration_ms: number;
    outcome: string;
  };
  estimated_usd: number | null;
  input_rate: number | null;
  cached_input_rate: number | null;
  output_rate: number | null;
  cached_rate_fallback: boolean;
};
