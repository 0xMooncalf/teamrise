import { useState } from "react";
import { ArrowDownToLine, Coins, Layers3, Timer, Workflow } from "lucide-react";
import AgentAvatar, { agentIdentity } from "./AgentAvatar";
import Tabs from "./Tabs";
import { duration } from "./shared";
import type { Candidate, State, UsageCall, SpeechCall } from "./types";

function SpeechUsage({calls}:{calls:SpeechCall[]}) {
  const recognition=calls.filter(c=>c.kind==="recognition"), synthesis=calls.filter(c=>c.kind==="synthesis");
  return <div className="report-block"><h4>Local speech processing</h4><div className="usage-breakdown"><span>Candidate audio<strong>{duration(recognition.reduce((n,c)=>n+c.audio_seconds,0))}</strong></span><span>Interviewer audio<strong>{duration(synthesis.reduce((n,c)=>n+c.audio_seconds,0))}</strong></span><span>Speech turns<strong>{recognition.length} in · {synthesis.length} out</strong></span></div><p className="soft-note">faster-whisper base.en + Piper ljspeech medium · local CPU. No speech API charge or token count is assigned. AI text usage is shown separately.</p>{calls.slice(-8).map((c,i)=><p className="fine-print" key={i}>{c.kind} · {c.audio_seconds.toFixed(1)}s audio · {(c.duration_ms/1000).toFixed(1)}s processing{c.characters?` · ${c.characters} characters`:""}</p>)}</div>
}

export function usd(value: number) {
  return new Intl.NumberFormat("en-US", {
    style: "currency",
    currency: "USD",
    minimumFractionDigits: 2,
    maximumFractionDigits: value > 0 && value < 0.01 ? 6 : 4,
  }).format(value);
}
export function tokenCount(value: number) {
  return new Intl.NumberFormat("en-US", {
    notation: value >= 100000 ? "compact" : "standard",
    maximumFractionDigits: 1,
  }).format(value);
}
export function summarize(calls: UsageCall[]) {
  const known = calls.filter((c) => c.usage.reported);
  const priced = calls.filter((c) => c.estimated_usd !== null);
  return {
    attempts: calls.length,
    reported: known.length,
    unknown: calls.length - known.length,
    priced: priced.length,
    subscription: calls.filter((c) => c.usage.billing === "subscription").length,
    input: known.reduce((n, c) => n + c.usage.input_tokens, 0),
    output: known.reduce((n, c) => n + c.usage.output_tokens, 0),
    cached: known.reduce((n, c) => n + c.usage.cached_tokens, 0),
    total: known.reduce((n, c) => n + c.usage.total_tokens, 0),
    cost: priced.reduce((n, c) => n + (c.estimated_usd ?? 0), 0),
  };
}
export function CandidateUsage({ candidate: c }: { candidate: Candidate }) {
  const calls = c.live?.calls ?? [];
  const sum = summarize(calls);
  return (
    <div className="candidate-usage">
      {c.live?.voice && <SpeechUsage calls={c.live.voice.calls || []}/>}
      <div className="usage-intro">
        <span className="eyebrow">INTERVIEW USAGE</span>
        <h3>
          {sum.reported ? tokenCount(sum.total) : "—"}{" "}
          <small>tokens reported</small>
        </h3>
        <p>
          {sum.attempts} model attempts ·{" "}
          {sum.priced
            ? usd(sum.cost) + " estimated"
            : sum.subscription === sum.attempts && sum.attempts > 0 ? "Codex subscription · no API invoice" : "Rates not configured or usage unavailable"}
          {sum.priced > 0 && sum.priced < sum.attempts
            ? " · partial total"
            : ""}
        </p>
      </div>
      <div className="usage-breakdown">
        <span>
          Input <strong>{sum.reported ? tokenCount(sum.input) : "—"}</strong>
        </span>
        <span>
          Cached input{" "}
          <strong>{sum.reported ? tokenCount(sum.cached) : "—"}</strong>
        </span>
        <span>
          Output <strong>{sum.reported ? tokenCount(sum.output) : "—"}</strong>
        </span>
      </div>
      {sum.unknown > 0 && (
        <p className="soft-note">
          Usage is unavailable for {sum.unknown} attempt(s). A timeout or failed
          response can still be billable.
        </p>
      )}
      {calls.length ? (
        <div className="usage-call-list">
          {calls.map((call) => (
            <article key={call.id}>
              <AgentAvatar agent={call.agent} size={30} />
              <div>
                <strong>
                  {agentIdentity(call.agent).nickname}{" "}
                  <small>{call.agent}</small>
                </strong>
                <p>
                  {call.usage.model} · {call.usage.outcome} ·{" "}
                  {(call.usage.duration_ms / 1000).toFixed(1)}s
                </p>
              </div>
              <div>
                <strong>
                  {call.usage.reported
                    ? tokenCount(call.usage.total_tokens) + " tokens"
                    : "Usage unavailable"}
                </strong>
                <p>
                  {call.estimated_usd !== null
                    ? usd(call.estimated_usd) + " estimated"
                    : call.usage.billing === "subscription" ? "Codex subscription" : "Cost unavailable"}
                </p>
              </div>
            </article>
          ))}
        </div>
      ) : (
        <div className="quiet-empty">
          <Coins size={28} />
          <h4>No model calls recorded yet</h4>
          <p>
            Real token counts appear after your provider responds. Earlier
            interviews have no retroactive usage data.
          </p>
        </div>
      )}
      <p className="fine-print">
        Codex subscription turns use your plan allowance; token counts are not a dollar invoice. {" "}
        Estimates use the USD rates saved with each request. Reasoning tokens
        are part of output, and cached tokens are part of input; neither is
        counted twice. Missing cached rates use the regular input rate. Provider
        invoices remain authoritative.
      </p>
    </div>
  );
}

export default function Insights({ state, demo }: { state: State; demo: boolean }) {
  const [tab, setTab] = useState("interviews");
  if (demo) {
    return (
      <section className="insights-panel" aria-label="Demo statistics">
        <div className="section-heading"><div><span className="eyebrow">DEMO SANDBOX</span><h2>Demo insights</h2></div>
          <a className="button secondary" href="/api/export?workspace=demo" download><ArrowDownToLine size={15} /> Export demo</a>
        </div>
          <div className="demo-insights">
            <h3>Simulation, on its own clock.</h3>
            <div className="usage-breakdown">
              <span>
                Interview time
                <strong>{duration(state.candidates.reduce((total, candidate) => total + candidate.interview_seconds, 0))}</strong>
              </span>
              <span>
                Elapsed demo time
                <strong>{duration(state.clock_seconds)}</strong>
              </span>
              <span>
                Completed interviews
                <strong>{state.candidates.filter((candidate) => candidate.interview_seconds > 0 && candidate.assessment).length}</strong>
              </span>
            </div>
            <p className="soft-note">
              These are modeled times from scripted scenarios. No model tokens
              or AI charges are generated by demo ticks.
            </p>
          </div>
      </section>
    );
  }
  const live = state.candidates;
  const calls = live.flatMap((c) => c.live?.calls ?? []);
  const totals = summarize(calls);
  return (
    <section className="insights-panel" aria-label="Interview statistics">
      <div className="section-heading">
        <div>
          <span className="eyebrow">EVERY CALL, ACCOUNTED FOR</span>
          <h2>Interview insights</h2>
          <p>Real usage, clear costs. Kept separate from the demo.</p>
        </div>
        <a className="button secondary" href="/api/export?workspace=live" download>
          <ArrowDownToLine size={15} />
          Export data
        </a>
      </div>
      <div className="insight-summary">
        <article>
          <Coins size={18} />
          <span>Estimated AI spend</span>
          <strong>{totals.priced ? usd(totals.cost) : "—"}</strong>
          <small>
            {totals.priced
              ? `${totals.priced}/${totals.attempts} attempts priced`
              : totals.subscription > 0 ? "Codex subscription usage is not priced" : "Add your provider’s rates in settings"}
          </small>
        </article>
        <article>
          <Layers3 size={18} />
          <span>Reported tokens</span>
          <strong>{totals.reported ? tokenCount(totals.total) : "—"}</strong>
          <small>
            {totals.reported}/{totals.attempts} attempts with usage
          </small>
        </article>
        <article>
          <Timer size={18} />
          <span>Real interview time</span>
          <strong>
            {duration(live.reduce((n, c) => n + c.interview_seconds, 0))}
          </strong>
          <small>
            {live.filter((c) => c.assessment).length} report(s) completed ·
            summed durations
          </small>
        </article>
      </div>
      <Tabs
        label="Statistics sections"
        value={tab}
        change={setTab}
        items={[
          { id: "interviews", label: "By interview" },
          { id: "agents", label: "By agent" },
          { id: "voice", label: "Speech" },
        ]}
      />
      <div className="tab-panel insight-content" key={tab} role="tabpanel">
        {tab === "interviews" ? (
          live.length ? (
            <div className="insight-table">
              <div className="insight-row table-labels">
                <span>Candidate / skill</span>
                <span>Tokens</span>
                <span>Estimated cost</span>
                <span>Duration</span>
              </div>
              {live.map((c) => {
                const s = summarize(c.live?.calls ?? []);
                return (
                  <div className="insight-row" key={c.id}>
                    <div>
                      <strong>{c.name}</strong>
                      <small>
                        {c.profile.name} · {s.attempts} attempts
                      </small>
                    </div>
                    <span>
                      {s.reported ? tokenCount(s.total) : "Unavailable"}
                      {s.unknown > 0 && <small>{s.unknown} unknown</small>}
                    </span>
                    <span>
                      {s.priced ? usd(s.cost) : s.subscription === s.attempts && s.attempts > 0 ? "Subscription" : "Unavailable"}
                      <small>
                        {s.priced > 0 && s.priced < s.attempts
                          ? "Partial estimate"
                          : ""}
                      </small>
                    </span>
                    <span>
                      {c.assessment
                        ? duration(c.interview_seconds)
                        : "In progress"}
                    </span>
                  </div>
                );
              })}
            </div>
          ) : (
            <div className="quiet-empty">
              <Workflow size={32} />
              <h3>Your first interview starts the story.</h3>
              <p>
                Token counts and costs appear here after real API requests. Demo
                scenarios do not consume AI tokens.
              </p>
            </div>
          )
        ) : tab === "voice" ? (<SpeechUsage calls={live.flatMap(c=>c.live?.voice?.calls || [])}/>) : tab === "agents" ? (
          <div className="agent-usage-grid">
            {state.agents.map((a) => {
              const s = summarize(calls.filter((c) => c.agent === a.name));
              return (
                <article key={a.name}>
                  <span style={{ color: a.color }}>
                    <AgentAvatar agent={a.name} size={42} />
                  </span>
                  <div>
                    <h3>
                      {agentIdentity(a.name).nickname}
                      <small>{a.name}</small>
                    </h3>
                    <p>
                      {s.attempts
                        ? `${s.attempts} model attempts${s.unknown ? ` · ${s.unknown} usage unavailable` : ""}`
                        : "Application workflow · no model calls"}
                    </p>
                  </div>
                  <strong>
                    {s.reported
                      ? tokenCount(s.total) + " tokens"
                      : s.attempts
                        ? "Usage unavailable"
                        : "—"}
                    <small>
                      {s.priced
                        ? usd(s.cost) +
                          (s.priced < s.attempts
                            ? " partial estimate"
                            : " estimated")
                        : s.attempts
                          ? s.subscription === s.attempts ? "Codex subscription" : "Cost unavailable"
                          : "No API charge recorded"}
                    </small>
                  </strong>
                </article>
              );
            })}
          </div>
        ) : (
          null
        )}
      </div>
      <p className="fine-print">
        Cost is a configured estimate, not an invoice. Missing usage is never
        presented as zero consumption. Connection-test calls are outside
        interview totals.
      </p>
    </section>
  );
}
