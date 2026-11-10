import GlassSelect from "./GlassSelect";
import { useState } from "react";
import {
  ArrowUpRight,
  Clock3,
  LockKeyhole,
  MessageSquare,
  Radio,
  Sparkles,
} from "lucide-react";
import { Avatar, duration, interviewFailed, StatusBadge } from "./shared";
import RobotTrail from "./RobotTrail";
import AgentAvatar, { agentIdentity } from "./AgentAvatar";
import Tabs from "./Tabs";
import { summarize, usd, tokenCount } from "./UsagePanel";
import type { State, Candidate, Action } from "./types";

export default function PersonCard({
  candidate: c,
  state,
  action,
  busy,
  open,
  room,
}: {
  candidate: Candidate;
  state: State;
  action: Action;
  busy: boolean;
  open: (id: string, tab?: string, agent?: number) => void;
  room: (id: string) => void;
}) {
  const [tab, setTab] = useState("snapshot");
  const [agent, setAgent] = useState(0);
  const live = c.mode === "live";
  const active = c.runs.find((r) => r.status === "active");
  const working = live
    ? c.live?.status === "generating" || c.live?.status === "analyzing"
    : state.running &&
      !!active &&
      !c.room.expired &&
      c.room.empty_since === null;
  const request = state.requests.find(
    (r) => r.candidate_id === c.id && r.status === "pending",
  );
  const total = summarize(c.live?.calls ?? []);
  const run = c.runs[agent];
  const aiScored = live ? c.live?.scored : c.score > 0;
  const status = live
    ? c.live?.status.replaceAll("_", " ")
    : c.room.expired
      ? "Room closed"
      : request
        ? "Human requested"
        : active
          ? `${agentIdentity(active.agent).nickname} ${state.running ? "is working" : "is paused"}`
          : "Ready for the next step";
  const failed = interviewFailed(c);
  const latest = [...c.transcript]
    .reverse()
    .find((message) => message.speaker !== "System");
  return (
    <article
      className={`person-card glass ${request ? "needs-review" : ""}`}
      aria-label={`Candidate ${c.name}`}
    >
      <header className="person-heading">
        <Avatar candidate={c} />
        <div className="person-identity">
          <h2>{c.name}</h2>
          <p>{c.profile.name || c.job}</p>
          <span className={`person-mode ${live ? "real" : ""}`}>
            {live ? "REAL AI" : "DEMO"}
          </span>
        </div>
        <div className={`person-score ${aiScored ? "scored" : ""}`}>
          <strong>{aiScored ? c.score : "—"}</strong>
          <span>/ 100</span>
        </div>
      </header>
      <div className="person-status">
        <StatusBadge
          tone={
            failed
              ? "failed"
              : working
                ? "active"
                : c.live?.status === "ready" || c.live?.status === "complete"
                  ? "ready"
                  : "quiet"
          }
        >
          {failed ? "Failed" : status}
        </StatusBadge>
        {!failed && (
          <span className="person-stage">
            {c.stage === "Sourced" ? "Queued" : c.stage}
          </span>
        )}
      </div>
      <RobotTrail
        candidate={c}
        agents={state.agents}
        running={!!working}
        select={(index) => {
          setAgent(index);
          setTab("agent");
        }}
      />
      <Tabs
        label={`Details for ${c.name}`}
        value={tab}
        change={setTab}
        items={[
          { id: "snapshot", label: "Overview" },
          { id: "conversation", label: "Interview" },
          { id: "report", label: "Report" },
          { id: "agent", label: "Team" },
        ]}
      />
      <div
        className="person-card-body tab-panel"
        key={tab + String(tab === "agent" ? agent : "")}
        role="tabpanel"
      >
        {tab === "snapshot" ? (
          <>
            <span className="eyebrow">
              {request
                ? "YOUR INPUT IS NEEDED"
                : failed
                  ? "INTERVIEW OUTCOME"
                  : "INTERVIEW SUMMARY"}
            </span>
            <h3>
              {c.live?.failure ||
                (request
                  ? request.reason
                  : c.assessment
                    ? "Evidence is ready for review."
                    : live
                      ? "A skill-led interview, at their pace."
                      : "A role-specific journey.")}
            </h3>
            <p className="clamp-three">
              {c.live?.error ||
                c.assessment?.summary ||
                (failed
                  ? "Open the interview to review the conversation saved so far."
                  : c.profile.skill_prompt)}
            </p>
            <div className="person-detail-line">
              <LockKeyhole size={12} />
              {live
                ? c.live?.discord
                  ? c.live?.voice
                    ? `Discord voice · ${c.live.voice.state.replaceAll("_", " ")}`
                    : "Discord permissions preview"
                  : "Web interview · answers supplied by a person"
                : "Simulated room · no external AI calls"}
            </div>
          </>
        ) : tab === "conversation" ? (
          <>
            <div className="preview-heading">
              <span className="eyebrow">LATEST MESSAGE</span>
              <span>{c.transcript.length} messages</span>
            </div>
            {latest ? (
              <>
                <strong className="message-speaker">
                  {latest.speaker === "Interviewer"
                    ? "Echo · Interviewer"
                    : c.name}
                </strong>
                <p className="clamp-four">{latest.text}</p>
                <span className="person-detail-line">
                  <MessageSquare size={12} />
                  Open the interview to read or play the full dialogue.
                </span>
              </>
            ) : (
              <div className="quiet-card-empty">
                <MessageSquare size={22} />
                <strong>A conversation is coming.</strong>
                <p>Questions and actual answers will appear here.</p>
              </div>
            )}
          </>
        ) : tab === "report" ? (
          c.assessment ? (
            <>
              <span className="eyebrow">AI RECOMMENDATION</span>
              <h3 className="clamp-two">{c.assessment.recommendation}</h3>
              <p className="clamp-three">{c.assessment.characterization}</p>
              <div className="person-detail-line">
                <Sparkles size={12} />
                Review the full evidence before deciding.
              </div>
            </>
          ) : (
            <div className="quiet-card-empty">
              <Sparkles size={22} />
              <strong>Evidence before conclusions.</strong>
              <p>The report will follow the interview.</p>
            </div>
          )
        ) : (
          <>
            <div
              className="person-agent-heading"
              style={{ color: state.agents[agent]?.color }}
            >
              <AgentAvatar agent={run.agent} size={35} />
              <div>
                <h3>
                  {agentIdentity(run.agent).nickname}
                  <span>{run.agent}</span>
                </h3>
                <small>{run.status}</small>
              </div>
            </div>
            <p className="clamp-three">
              {run.summary || state.agents[agent]?.role}
            </p>
            <div className="person-detail-line">
              {run.agent === "Interviewer"
                ? "Candidate-facing interviewer"
                : run.agent === "Analyst"
                  ? "Internal analysis · no Discord presence"
                  : "Internal workflow · no separate Discord bot"}
            </div>
          </>
        )}
      </div>
      <footer className="person-card-footer">
        <div className="person-footnote">
          <Clock3 size={12} />
          {c.assessment || !live
            ? duration(c.interview_seconds)
            : "Not completed"}
          {live && total.attempts > 0 && (
            <span title={`${total.unknown} attempt(s) with unavailable usage`}>
              ·{" "}
              {total.reported
                ? tokenCount(total.total) + " tokens"
                : "Usage pending"}
              {total.priced ? " · " + usd(total.cost) : ""}
            </span>
          )}
        </div>
        <div className="person-card-actions">
          {!live && c.phase === 0 && c.runs[0]?.status === "idle" && (
            <GlassSelect
              label={`Interview skill for ${c.name}`}
              value={c.profile_id}
              disabled={busy}
              onChange={(value) =>
                void action(`/api/candidates/${c.id}/profile`, {
                  profile_id: value,
                })
              }
              options={state.profiles.map((p) => ({
                value: p.id,
                label: p.name,
              }))}
            />
          )}
          {!live && (c.room.bot_present || c.room.expired) && (
            <button
              className="icon-button"
              aria-label={`Open room for ${c.name}`}
              title="Demo room controls"
              onClick={() => room(c.id)}
            >
              <Radio size={15} />
            </button>
          )}
          <button
            className={`button ${request ? "amber" : "secondary"}`}
            onClick={() =>
              open(
                c.id,
                tab === "conversation"
                  ? "Interview"
                  : tab === "report"
                    ? "AI report"
                    : tab === "agent"
                      ? "Agents"
                      : "Overview",
                agent,
              )
            }
          >
            {live ? "Open interview" : "View profile"}
            <ArrowUpRight size={14} />
          </button>
        </div>
      </footer>
    </article>
  );
}
