import GlassSelect from "./GlassSelect";
import { useEffect, useId, useState } from "react";
import {
  ArrowRight,
  Clock3,
  LockKeyhole,
  MessageSquare,
  Mic,
  Radio,
  RotateCw,
  Send,
  Sparkles,
} from "lucide-react";
import {
  Modal,
  Avatar,
  duration,
  StatusBadge,
  interviewFailed,
} from "./shared";
import { Transcript, AIReport } from "./CandidateCard";
import Tabs from "./Tabs";
import AgentAvatar, { agentIdentity } from "./AgentAvatar";
import RobotTrail from "./RobotTrail";
import { ChannelBadge } from "./ConnectionsPanel";
import { CandidateUsage } from "./UsagePanel";
import type { State, Candidate, Action } from "./types";
import type { Speech } from "./speech";

function InterviewClock({ deadline }: { deadline: string }) {
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const interval = window.setInterval(() => setNow(Date.now()), 1000);
    return () => window.clearInterval(interval);
  }, []);
  const seconds = Math.max(0, Math.ceil((Date.parse(deadline) - now) / 1000));
  const h = Math.floor(seconds / 3600);
  const m = String(Math.floor(seconds / 60) % 60).padStart(2, "0");
  const s = String(seconds % 60).padStart(2, "0");
  return (
    <span
      className={seconds <= 60 ? "interview-clock urgent" : "interview-clock"}
      role="timer"
      aria-label="Interview time remaining"
    >
      <Clock3 size={13} /> {h ? h + ":" : ""}
      {m}:{s} left
    </span>
  );
}

const voiceStates = {
  listening: {
    label: "Waiting for a spoken answer",
    icon: Mic,
    detail:
      "After five minutes of silence, Echo reconnects and repeats the question.",
  },
  awaiting_chat: {
    label: "Waiting for a written answer",
    icon: MessageSquare,
    detail:
      "The candidate can submit their answer in the room chat with /interview-answer.",
  },
  recovering_voice: {
    label: "Reconnecting voice",
    icon: RotateCw,
    detail: "Echo will resume the current question in the same room.",
  },
  waiting_for_people: {
    label: "Waiting for the candidate",
    icon: Radio,
    detail: "The interview resumes when the candidate is alone with Echo.",
  },
  transcribing: { label: "Transcribing the answer", icon: Mic, detail: "" },
  speaking: { label: "Echo is speaking", icon: Mic, detail: "" },
  thinking: { label: "Preparing the next question", icon: Radio, detail: "" },
  voice_error: { label: "Voice interrupted", icon: Radio, detail: "" },
  human_review: { label: "Ready for human review", icon: Radio, detail: "" },
  stopped: { label: "Interview stopped", icon: Radio, detail: "" },
  closed: { label: "Discord room closed", icon: Radio, detail: "" },
};

function VoiceStatus({ candidate }: { candidate: Candidate }) {
  const live = candidate.live;
  const voice = live?.voice;
  if (!voice) return null;
  const {
    label,
    icon: Icon,
    detail,
  } = voiceStates[voice.state as keyof typeof voiceStates] ?? {
    label: voice.state.replaceAll("_", " "),
    icon: Radio,
    detail: "",
  };
  const message = live.failure || voice.error || detail;
  return (
    <div className="voice-summary">
      <div className="voice-summary-copy" role="status">
        <Icon className="voice-summary-icon" size={18} aria-hidden="true" />
        <div>
          <strong>{label}</strong>
          {message && <p>{message}</p>}
          {voice.empty_deadline && (
            <p>
              Room closes at{" "}
              {new Date(voice.empty_deadline).toLocaleTimeString()} if the
              candidate is still away.
            </p>
          )}
        </div>
      </div>
      <ChannelBadge candidate={candidate} />
    </div>
  );
}

export function CreateInterview({
  state,
  created,
  close,
}: {
  state: State;
  created: (state: State, id: string) => void;
  close: () => void;
}) {
  const [name, setName] = useState("");
  const [profile, setProfile] = useState(state.profiles[0]?.id || "");
  const [prepareRoom, setPrepareRoom] = useState(false);
  const [member, setMember] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  async function create(e: React.FormEvent) {
    e.preventDefault();
    setBusy(true);
    setError("");
    try {
      const res = await fetch("/api/interviews", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          name,
          profile_id: profile,
          discord_member_id: prepareRoom ? member : "",
        }),
      });
      const s = await res.json();
      if (!res.ok) throw Error(s.error);
      created(s, s.created_candidate_id);
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not create interview.");
    } finally {
      setBusy(false);
    }
  }
  const formID = useId();
  return (
    <Modal
      title="A new conversation"
      close={close}
      footer={
        <div className="form-actions">
          <button
            type="submit"
            form={formID}
            className="button primary"
            disabled={busy || !profile}
          >
            {busy ? "Creating…" : "Create interview"}
            <ArrowRight size={16} />
          </button>
        </div>
      }
    >
      <form id={formID} className="modal-body live-create" onSubmit={create}>
        <p className="modal-description">
          Choose a skill. Echo interviews the candidate; the rest of the team
          works behind the scenes.
        </p>
        <label className="form-field">
          <span>Candidate name</span>
          <input
            required
            minLength={2}
            maxLength={100}
            value={name}
            onChange={(e) => setName(e.target.value)}
            placeholder="e.g. Alex Morgan"
            autoFocus
          />
        </label>
        <label className="form-field">
          <span>Interview skill & prompt</span>
          <GlassSelect
            label="Interview skill & prompt"
            value={profile}
            onChange={setProfile}
            options={state.profiles.map((p) => ({
              value: p.id,
              label: p.name,
            }))}
          />
        </label>
        <div className="live-prompt-preview">
          {state.profiles.find((p) => p.id === profile)?.skill_prompt}
        </div>
        {state.connections.discord.enabled && (
          <>
            <label className="connection-check">
              <input
                type="checkbox"
                checked={prepareRoom}
                onChange={(e) => setPrepareRoom(e.target.checked)}
              />
              Preview Discord room permissions (web interview)
            </label>
            {prepareRoom && (
              <label className="form-field">
                <span>Candidate Discord member ID</span>
                <input
                  required
                  pattern="[0-9]{17,20}"
                  value={member}
                  onChange={(e) => setMember(e.target.value.trim())}
                  placeholder="Only this candidate will be invited"
                />
                <small>
                  This previews permissions only. For a real voice interview,
                  run /interview in Discord and accept the interview notice.
                </small>
              </label>
            )}
          </>
        )}
        <p className="soft-note">
          Actual answers are entered by a person. Questions and reports are sent
          to your selected AI provider.
        </p>
        {!state.connections.ai.model && (
          <p className="connection-result">
            Choose a model in Connections before beginning.
          </p>
        )}
        {error && (
          <p role="alert" className="connection-result failed">
            {error}
          </p>
        )}
      </form>
    </Modal>
  );
}

export default function LiveInterview({
  candidate: c,
  state,
  speech,
  adminID,
  action,
  sync,
  close,
  initialTab = "conversation",
}: {
  candidate: Candidate;
  state: State;
  speech: Speech;
  adminID: string;
  action: Action;
  sync: (s: State) => void;
  close: () => void;
  initialTab?: string;
}) {
  const [answer, setAnswer] = useState("");
  const [sending, setSending] = useState(false);
  const [error, setError] = useState("");
  const [tab, setTab] = useState(initialTab);
  const [earlier, setEarlier] = useState(false);
  const [agent, setAgent] = useState(1);
  const l = c.live!;
  const voice = l.voice;
  const active = l.status === "generating" || l.status === "analyzing";
  const request = state.requests.find(
    (r) => r.candidate_id === c.id && r.status === "pending",
  );
  const run = c.runs[agent];
  async function runAI(kind: string) {
    setSending(true);
    setError("");
    try {
      const res = await fetch(`/api/candidates/${c.id}/ai/${kind}`, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({
          answer: kind === "answer" ? answer : "",
          version: l.version,
        }),
      });
      const result: State & { error?: string } = await res.json();
      if (!res.ok) throw Error(result.error);
      sync(result);
      if (kind === "answer") setAnswer("");
      if (result.candidates.find((p) => p.id === c.id)?.assessment)
        setTab("report");
    } catch (e) {
      setError(e instanceof Error ? e.message : "Could not contact Teamrise.");
    } finally {
      setSending(false);
    }
  }
  return (
    <Modal
      title="Interview workspace"
      close={close}
      wide
      scrollKey={tab}
      toolbar={
        <>
          <div className="interview-person">
            <Avatar candidate={c} />
            <div>
              <h2>{c.name}</h2>
              <p>
                {c.profile.name} ·{" "}
                {l.model ||
                  state.connections.ai.model ||
                  "Model not configured"}
              </p>
            </div>
            <StatusBadge
              tone={
                interviewFailed(c)
                  ? "failed"
                  : active
                    ? "active"
                    : l.status === "ready" || l.status === "complete"
                      ? "ready"
                      : "quiet"
              }
            >
              {interviewFailed(c) ? "Failed" : l.status.replaceAll("_", " ")}
            </StatusBadge>
          </div>
          <Tabs
            label="Interview workspace sections"
            value={tab}
            change={setTab}
            items={[
              { id: "conversation", label: "Conversation" },
              { id: "report", label: "Assessment" },
              { id: "team", label: "Team" },
              { id: "skill", label: "Skill" },
              { id: "usage", label: "Usage" },
              { id: "room", label: "Room" },
            ]}
          />
        </>
      }
      footer={
        <footer className="interview-footer">
          <span>
            {c.assessment
              ? duration(c.interview_seconds) + " actual duration"
              : `${c.transcript.filter((m) => m.speaker === "Candidate").length} answer(s) collected`}
          </span>
          {l.deadline &&
            ["awaiting_answer", "generating", "error"].includes(l.status) && (
              <InterviewClock deadline={l.deadline} />
            )}
          <span>
            <LockKeyhole size={12} />
            Private interview record
          </span>
        </footer>
      }
    >
      <div
        className="interview-workspace-body tab-panel"
        key={tab}
        role="tabpanel"
      >
        {tab === "conversation" ? (
          <>
            {c.transcript.length > 4 && (
              <button
                className="text-button earlier-messages"
                onClick={() => setEarlier(!earlier)}
              >
                {earlier
                  ? "Show latest messages"
                  : `Show ${c.transcript.length - 4} earlier messages`}
              </button>
            )}
            {voice && <VoiceStatus candidate={c} />}
            <Transcript
              candidate={c}
              speech={speech}
              maxMessages={earlier ? Infinity : 4}
            />
            {(error || l.error) && (
              <p className="connection-result failed" role="alert">
                {error || l.error}
              </p>
            )}
            {active && (
              <p className="connection-result" role="status">
                {l.status === "analyzing"
                  ? "Prism is analyzing the answers…"
                  : "Echo is preparing the next question…"}{" "}
                You can monitor another interview while this runs.
              </p>
            )}
            {!voice && l.status === "ready" && (
              <div className="live-composer">
                <p>
                  The saved skill and question plan will be sent to your model.
                  A person supplies the answers.
                </p>
                <button
                  className="button primary"
                  disabled={sending || !state.connections.ai.model}
                  onClick={() => runAI("start")}
                >
                  <Sparkles size={16} />
                  Begin interview
                </button>
              </div>
            )}
            {!voice && l.status === "awaiting_answer" && (
              <form
                className="live-composer"
                onSubmit={(e) => {
                  e.preventDefault();
                  runAI("answer");
                }}
              >
                <label className="form-field">
                  <span>Candidate’s answer</span>
                  <textarea
                    value={answer}
                    onChange={(e) => setAnswer(e.target.value)}
                    placeholder="Type the candidate’s actual answer…"
                    maxLength={16000}
                    rows={3}
                    required
                  />
                </label>
                <div>
                  <span>One question at a time · take your time</span>
                  <button
                    type="button"
                    className="button secondary"
                    disabled={
                      sending ||
                      !c.transcript.some((m) => m.speaker !== "Interviewer")
                    }
                    onClick={() => runAI("analyze")}
                  >
                    Finish & analyze
                  </button>
                  <button
                    className="button primary"
                    disabled={sending || !answer.trim()}
                  >
                    <Send size={15} />
                    Send answer
                  </button>
                </div>
              </form>
            )}
            {!voice && l.status === "error" && (
              <div className="live-composer">
                <p>
                  The saved answer is retained. Retry continues the failed step.
                </p>
                <button
                  className="button primary"
                  disabled={sending}
                  onClick={() => runAI("retry")}
                >
                  Retry AI request
                </button>
              </div>
            )}
          </>
        ) : tab === "report" ? (
          <>
            <AIReport candidate={c} />
            {!!l.limitations?.length && (
              <div className="report-block">
                <h4>Limitations & missing evidence</h4>
                <ul>
                  {l.limitations.map((s, i) => (
                    <li key={i}>{s}</li>
                  ))}
                </ul>
              </div>
            )}
            {c.assessment && (
              <div className="live-human-review">
                <div>
                  <h4>
                    {c.stage === "Hired" || c.stage === "Rejected"
                      ? `Human decision: ${c.stage}`
                      : "A human has the final word."}
                  </h4>
                  <p>
                    {l.reviewer
                      ? `Review claimed by ${l.reviewer}.`
                      : voice
                        ? "Join the Discord room with the Reviewer role. Actual entry confirms attendance on every dashboard."
                        : "Claim the review to record a decision. This is a web review, not a Discord join."}
                  </p>
                </div>
                {request && !voice && (
                  <button
                    className="button amber"
                    onClick={() =>
                      action(`/api/requests/${request.id}`, {
                        action: "acknowledge",
                        admin_id: adminID,
                      })
                    }
                  >
                    Claim web review
                  </button>
                )}
                {l.reviewer && (
                  <div className="form-actions">
                    {c.stage === "Review" && (
                      <button
                        className="button secondary"
                        onClick={() =>
                          action(`/api/candidates/${c.id}/decision`, {
                            action: "resolve",
                          })
                        }
                      >
                        Move to final review
                      </button>
                    )}
                    {c.stage === "Final" && (
                      <button
                        className="button primary"
                        onClick={() =>
                          action(`/api/candidates/${c.id}/decision`, {
                            action: "hire",
                          })
                        }
                      >
                        Record hire decision
                      </button>
                    )}
                    {(c.stage === "Review" || c.stage === "Final") && (
                      <button
                        className="button secondary"
                        onClick={() =>
                          action(`/api/candidates/${c.id}/decision`, {
                            action: "reject",
                          })
                        }
                      >
                        Record rejection
                      </button>
                    )}
                  </div>
                )}
              </div>
            )}
          </>
        ) : tab === "usage" ? (
          <CandidateUsage candidate={c} />
        ) : tab === "team" ? (
          <>
            <RobotTrail
              candidate={c}
              agents={state.agents}
              running={active}
              select={setAgent}
            />
            <div
              className="agent-detail tab-panel"
              key={agent}
              style={
                {
                  "--agent-color": state.agents[agent]?.color,
                } as React.CSSProperties
              }
            >
              <span className="section-label">
                <AgentAvatar agent={run.agent} size={35} />
                {agentIdentity(run.agent).nickname} · {run.agent}
              </span>
              <h3>{state.agents[agent]?.role}</h3>
              <p>{run.summary || agentIdentity(run.agent).personality}</p>
              <span className="status-pill neutral">{run.status}</span>
              <p className="fine-print">
                {run.agent === "Interviewer"
                  ? "The only candidate-facing voice. Other agents do not join the room."
                  : "Internal workflow role. No individual Discord channel or account is required."}
              </p>
            </div>
          </>
        ) : tab === "skill" ? (
          <>
            <span className="eyebrow">PINNED TO THIS INTERVIEW</span>
            <h3>{c.profile.name}</h3>
            <pre className="prompt-preview">{c.profile.skill_prompt}</pre>
            <h4>Assessment criteria</h4>
            <ul className="question-plan">
              {c.profile.criteria.map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ul>
            <h4>Question plan</h4>
            <ol className="question-plan">
              {c.profile.questions.map((s) => (
                <li key={s}>{s}</li>
              ))}
            </ol>
          </>
        ) : (
          <div className="private-room-info">
            <span className="integration-icon">
              <LockKeyhole size={25} />
            </span>
            <h3>
              {l.discord
                ? "A private room, prepared automatically."
                : "This conversation takes place on the web."}
            </h3>
            <ChannelBadge candidate={c} />
            <p>
              {l.discord
                ? "The candidate and interviewer are explicitly allowed. Ordinary server members cannot view or join the planned room. The reviewer role and server Administrators retain access."
                : "No Discord room has been assigned. Configure the common private-room policy for future interviews."}
            </p>
            {l.discord && (
              <div className="usage-breakdown">
                <span>
                  Room name<strong>{l.discord.payload.name}</strong>
                </span>
                <span>
                  Access rules
                  <strong>
                    {l.discord.payload.permission_overwrites.length} overwrites
                  </strong>
                </span>
                <span>
                  Connection
                  <strong>
                    {voice
                      ? voice.state.replaceAll("_", " ")
                      : "Permission preview only"}
                  </strong>
                </span>
              </div>
            )}
            <p className="soft-note">
              {voice
                ? "This is an actual Discord voice room. The AI pauses for human conversation when another person enters. The room is removed after the candidate is absent for five minutes. Returning cancels the timer, including after the AI report is finished."
                : "This web interview has no live Discord audio. To start a private voice room, enable voice in Connections and run /interview in Discord."}
            </p>
          </div>
        )}
      </div>
    </Modal>
  );
}
