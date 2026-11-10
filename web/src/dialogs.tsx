import GlassSelect from "./GlassSelect";
import { useState } from "react";
import {
  AudioLines,
  Bot,
  Check,
  Clock3,
  Copy,
  ExternalLink,
  LogIn,
  LogOut,
  Plus,
  Radio,
  Save,
  ShieldAlert,
  Sparkles,
  UserRound,
  Users,
  WandSparkles,
} from "lucide-react";
import { AIReport, Transcript } from "./CandidateCard";
import RobotTrail from "./RobotTrail";
import Tabs from "./Tabs";
import AgentAvatar, { agentIdentity } from "./AgentAvatar";
import { Avatar, Modal, countdown, duration, stageTone } from "./shared";
import type { Action, Candidate, InterviewProfile, State } from "./types";
import type { Speech } from "./speech";

export function VoiceSettings({
  speech,
  close,
}: {
  speech: Speech;
  close: () => void;
}) {
  const choices = speech.voices.filter((v) => /^(en|ru)/i.test(v.lang));
  return (
    <Modal title="A clearer conversation" close={close}>
      <div className="modal-body">
        <div className="voice-illustration">
          <AudioLines size={38} />
        </div>
        <p className="modal-description">
          Play both sides of the interview at a calmer pace, or listen to just
          one message. Use the play button next to any reply.
        </p>
        <label className="form-field">
          Speaking speed <strong>{speech.prefs.rate.toFixed(2)}×</strong>
          <input
            type="range"
            min=".6"
            max="1.2"
            step=".02"
            value={speech.prefs.rate}
            onChange={(e) =>
              speech.setPrefs({ ...speech.prefs, rate: Number(e.target.value) })
            }
          />
          <small>Slower and clearer by default · 0.78×</small>
        </label>
        <label className="form-field">
          Volume
          <input
            type="range"
            min="0"
            max="1"
            step=".05"
            value={speech.prefs.volume}
            onChange={(e) =>
              speech.setPrefs({
                ...speech.prefs,
                volume: Number(e.target.value),
              })
            }
          />
        </label>
        {(["interviewer", "candidate"] as const).map((role) => (
          <label key={role} className="form-field">
            {role === "interviewer" ? "Interviewer voice" : "Candidate voice"}
            <GlassSelect
              label={
                role === "interviewer" ? "Interviewer voice" : "Candidate voice"
              }
              value={speech.prefs[role]}
              onChange={(value) =>
                speech.setPrefs({ ...speech.prefs, [role]: value })
              }
              options={[
                { value: "", label: "System default" },
                ...choices.map((v) => ({
                  value: v.voiceURI,
                  label: v.name + " · " + v.lang,
                })),
              ]}
            />
          </label>
        ))}
        <button
          className="button primary"
          disabled={!speech.supported}
          onClick={() =>
            speech.playing === "voice-preview"
              ? speech.stop()
              : speech.play(
                  [
                    {
                      speaker: "Interviewer",
                      text: "Take your time. Could you walk me through your approach?",
                      response_seconds: 0,
                    },
                    {
                      speaker: "Candidate",
                      text: "Of course. I would start by clarifying the task, then explain each step.",
                      response_seconds: 0,
                    },
                  ],
                  "voice-preview",
                  "Candidate",
                )
          }
        >
          <AudioLines size={16} />
          {speech.playing === "voice-preview"
            ? "Stop voice preview"
            : "Preview both voices"}
        </button>
        {!speech.supported && (
          <div className="inline-notice">
            Speech synthesis is unavailable in this browser.
          </div>
        )}
        <p className="fine-print">
          Voice quality and available voices depend on your browser and
          operating system. This is playback of demo text, not a live voice
          connection.
        </p>
      </div>
    </Modal>
  );
}

export function CandidateInspector({
  candidate: c,
  state,
  speech,
  close,
  initialTab = "Overview",
  initialAgent = 0,
}: {
  candidate: Candidate;
  state: State;
  speech: Speech;
  close: () => void;
  initialTab?: string;
  initialAgent?: number;
}) {
  const [tab, setTab] = useState(initialTab);
  const [agent, setAgent] = useState(initialAgent);
  const run = c.runs[agent];
  return (
    <Modal
      title="Candidate workspace"
      close={close}
      wide
      scrollKey={tab}
      toolbar={
        <>
          <div className="inspector-person">
            <Avatar candidate={c} large />
            <div>
              <h2>{c.name}</h2>
              <p>
                {c.job} · {c.room.name}
              </p>
            </div>
            <span className={`status-pill ${stageTone(c.stage)}`}>
              {c.stage}
            </span>
          </div>
          <Tabs
            label="Candidate profile sections"
            value={tab}
            change={setTab}
            items={[
              "Overview",
              "Interview",
              "AI report",
              "Agents",
              "Skill prompt",
            ].map((t) => ({ id: t, label: t }))}
          />
        </>
      }
    >
      <div className="inspector-body tab-panel" key={tab} role="tabpanel">
        {tab === "Interview" ? (
          <Transcript candidate={c} speech={speech} />
        ) : tab === "AI report" ? (
          <AIReport candidate={c} />
        ) : tab === "Agents" ? (
          <>
            <RobotTrail
              candidate={c}
              agents={state.agents}
              running={state.running}
              select={setAgent}
            />
            <section
              key={agent}
              className="agent-detail"
              style={
                {
                  "--agent-color": state.agents[agent]?.color,
                } as React.CSSProperties
              }
            >
              <span className="section-label">
                <AgentAvatar agent={run.agent} size={30} />
                {agentIdentity(run.agent).nickname.toUpperCase()} ·{" "}
                {run.agent.toUpperCase()}
              </span>
              <h3>{state.agents[agent]?.role}</h3>
              <p className="agent-personality">
                {agentIdentity(run.agent).personality}
              </p>
              <span
                className={`status-pill ${run.status === "done" ? "green" : run.status === "active" ? "blue" : "neutral"}`}
              >
                {run.status}
              </span>
              <p>
                {run.summary ||
                  "This agent has not completed an assignment for this candidate yet."}
              </p>
            </section>
          </>
        ) : tab === "Skill prompt" ? (
          <>
            <span className="status-pill purple">{c.profile.name}</span>
            <h3 className="content-heading">Pinned specialist prompt</h3>
            <pre className="prompt-preview">{c.profile.skill_prompt}</pre>
            <h3 className="content-heading">Question plan</h3>
            <ol className="question-plan">
              {c.profile.questions.map((q) => (
                <li key={q}>{q}</li>
              ))}
            </ol>
            <p className="fine-print">
              Profile edits apply to future interviews. This candidate keeps the
              version pinned at the start.
            </p>
          </>
        ) : (
          <>
            <div className="inspector-stats">
              <div>
                <Clock3 size={17} />
                <strong>{duration(c.interview_seconds)}</strong>
                <span>modeled interview time</span>
              </div>
              <div>
                <Users size={17} />
                <strong>{c.room.participants.length}</strong>
                <span>participants in room</span>
              </div>
              <div>
                <Sparkles size={17} />
                <strong>
                  {c.score || "—"}
                  <small>/100</small>
                </strong>
                <span>synthetic score</span>
              </div>
            </div>
            {c.flag && (
              <div className="warning-box">
                <ShieldAlert size={20} />
                <div>
                  <strong>
                    {c.review_resolved
                      ? "Human reviewed this flag"
                      : "Human review requested"}
                  </strong>
                  <p>{c.flag}</p>
                </div>
              </div>
            )}
            <AIReport candidate={c} />
            {c.feedback.length > 0 && (
              <>
                <h3 className="content-heading">Feedback</h3>
                <ul className="question-plan">
                  {c.feedback.map((f) => (
                    <li key={f}>{f}</li>
                  ))}
                </ul>
              </>
            )}
          </>
        )}
      </div>
    </Modal>
  );
}

export function RoomMonitor({
  candidate: c,
  state,
  adminID,
  action,
  busy,
  close,
}: {
  candidate: Candidate;
  state: State;
  adminID: string;
  action: Action;
  busy: boolean;
  close: () => void;
}) {
  const [notice, setNotice] = useState("");
  const requests = state.requests.filter((r) => r.candidate_id === c.id);
  const request =
    [...requests].reverse().find((r) => r.status === "pending") ??
    requests.at(-1);
  const me = c.room.participants.find((p) => p.id === adminID);
  const remaining =
    c.room.reconnect_deadline === null
      ? null
      : c.room.reconnect_deadline - state.clock_seconds;
  async function roomAction(name: string) {
    if (
      !(await action(`/api/candidates/${c.id}/room`, {
        action: name,
        admin_id: adminID,
      }))
    )
      setNotice(
        "This room changed. Check its current participants and status.",
      );
    else setNotice("Room presence updated for every admin dashboard.");
  }
  return (
    <Modal title={`Room · ${c.room.name}`} close={close} wide>
      <div className="room-monitor">
        <div className="room-topline">
          <span className="demo-tag">
            <span />
            SIMULATED VOICE ROOM
          </span>
          <span
            className={`status-pill ${c.room.expired ? "neutral" : remaining !== null ? "amber" : "green"}`}
          >
            {c.room.expired
              ? "Bot left"
              : remaining !== null
                ? `Reconnect ${countdown(remaining)}`
                : "Bot stays connected"}
          </span>
        </div>
        <div className="room-stage">
          <div className={`room-bot ${c.room.bot_present ? "present" : ""}`}>
            <Bot size={37} />
            <strong>Sol</strong>
            <span>
              {c.room.bot_present ? "Still in the channel" : "Left the channel"}
            </span>
          </div>
          {c.room.participants.map((p) => (
            <div className="room-person" key={p.id}>
              <span>
                {p.kind === "admin" ? (
                  <ShieldAlert size={29} />
                ) : (
                  <UserRound size={29} />
                )}
              </span>
              <strong>{p.name}</strong>
              <small>{p.kind === "admin" ? "Human admin" : "Candidate"}</small>
            </div>
          ))}
          {!c.room.participants.some((p) => p.kind === "candidate") && (
            <div className="room-empty">
              <LogOut size={28} />
              <strong>Candidate is away</strong>
              <p>
                {c.room.expired
                  ? "The 5-minute window ended. Sol left the channel."
                  : "Sol is waiting for the candidate to return. Reviewer attendance does not reset the five-minute timer."}
              </p>
            </div>
          )}
        </div>
        {request?.status === "pending" && (
          <div className="room-request">
            <div>
              <span className="section-label">SHARED HUMAN REQUEST</span>
              <h3>{request.reason}</h3>
              <p>
                Opening this room does not dismiss the request. A confirmed
                entry event does.
              </p>
            </div>
            <button
              className="button primary"
              disabled={busy || !c.room.bot_present}
              onClick={async () => {
                const ok = await action(`/api/requests/${request.id}`, {
                  action: "confirm",
                  admin_id: adminID,
                });
                setNotice(
                  ok
                    ? "Entry confirmed. This request is now dimmed and disabled for all admins."
                    : "The request changed; another admin may have joined first.",
                );
              }}
            >
              <LogIn size={16} />
              Confirm my entry (demo)
            </button>
          </div>
        )}
        {request && request.status !== "pending" && (
          <div className="confirmed-entry">
            <Check size={17} />
            <span>
              {request.attended_by
                ? `${request.attended_by} entered · request ${request.status === "closed" ? "resolved" : "attended"}`
                : "Request closed after the room ended"}
            </span>
          </div>
        )}
        {me && (c.stage === "Final" || c.stage === "Review") && (
          <div className="human-decisions">
            <div>
              <Users size={17} />
              <strong>Your human decision</strong>
            </div>
            <button
              className="button secondary"
              disabled={busy}
              onClick={() =>
                action(`/api/candidates/${c.id}/decision`, { action: "reject" })
              }
            >
              Decline
            </button>
            <button
              className="button primary"
              disabled={busy}
              onClick={() =>
                action(`/api/candidates/${c.id}/decision`, {
                  action: c.stage === "Review" ? "resolve" : "hire",
                })
              }
            >
              {c.stage === "Review" ? "Clear flag & continue" : "Approve hire"}
            </button>
          </div>
        )}
        {c.feedback.length > 0 && (
          <div className="inline-notice">{c.feedback.join(" ")}</div>
        )}
        <section className="room-demo-controls">
          <div>
            <h3>Test channel presence</h3>
            <p>
              These controls simulate voice-state events. All admins see the
              same room state.
            </p>
          </div>
          <div className="room-actions">
            <button
              className="button secondary"
              disabled={busy || !c.room.bot_present}
              onClick={() =>
                roomAction(
                  c.room.participants.some((p) => p.kind === "candidate")
                    ? "candidate-leave"
                    : "candidate-rejoin",
                )
              }
            >
              {c.room.participants.some((p) => p.kind === "candidate") ? (
                <LogOut size={14} />
              ) : (
                <LogIn size={14} />
              )}
              Candidate{" "}
              {c.room.participants.some((p) => p.kind === "candidate")
                ? "leaves"
                : "rejoins"}
            </button>
            <button
              className="button secondary"
              disabled={busy || !me || !c.room.bot_present}
              onClick={() => roomAction("admin-leave")}
            >
              I leave the room
            </button>
            <button
              className="button secondary"
              disabled={
                busy || !c.room.participants.length || !c.room.bot_present
              }
              onClick={() => roomAction("everyone-leave")}
            >
              Everyone leaves
            </button>
          </div>
          <div className="clock-controls">
            <Clock3 size={16} />
            <span>Demo clock · {duration(state.clock_seconds)}</span>
            <button
              disabled={busy}
              onClick={() => action("/api/clock", { seconds: 60 })}
            >
              +1 minute
            </button>
            <button
              disabled={busy}
              onClick={() => action("/api/clock", { seconds: 300 })}
            >
              +5 minutes
            </button>
          </div>
        </section>
        {notice && (
          <div className="inline-notice" role="status">
            {notice}
          </div>
        )}
        <p className="fine-print">
          The bot never leaves just because an admin joins or records a
          decision. It leaves after the candidate is absent for 5 simulated
          minutes, even if a reviewer remains. The clock freezes when the demo
          is paused; use the time controls to test the timeout.
        </p>
      </div>
    </Modal>
  );
}
