import GlassSelect from "./GlassSelect";
import React, { lazy, Suspense, useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  Activity,
  ArrowDownToLine,
  ArrowRight,
  AudioLines,
  Bell,
  ChartNoAxesCombined,
  Check,
  Coins,
  FileSliders,
  LayoutGrid,
  Moon,
  Pause,
  Play,
  Plus,
  RotateCcw,
  Search,
  Settings2,
  SkipForward,
  Square,
  Sun,
  Users,
  X,
} from "lucide-react";
import PersonCard from "./PersonCard";
import ConnectionsPanel from "./ConnectionsPanel";
import SkillEditor from "./SkillEditor";
import LiveInterview, { CreateInterview } from "./LiveInterview";
import Insights, { summarize, usd } from "./UsagePanel";
import ReviewQueue, { reviewerNames } from "./ReviewQueue";
import Tabs from "./Tabs";
import { CandidateInspector, RoomMonitor, VoiceSettings } from "./dialogs";
import { Modal, SolMark, duration, interviewFailed } from "./shared";
import { readLocal, useSpeech } from "./speech";
import type { Action, State } from "./types";
import "./workspace.css";
import "./connections.css";
import "./glass.css";
import "./interface.css";
import "./modal.css";
import "./interview-tools.css";

const DiscordPreview = lazy(() => import("./DiscordPreview"));
type Detail = { id: string; tab?: string; agent?: number };

// Engine and connection revisions are independent. A late response must not
// roll back settings already received through another tab's SSE update.
function mergeState(current: State | undefined, incoming: State): State {
  if (!current) return incoming;
  const engine = incoming.revision >= current.revision ? incoming : current;
  const connections =
    incoming.connections.revision >= current.connections.revision
      ? incoming.connections
      : current.connections;
  const discord_connection =
    (incoming.discord_connection?.revision ?? 0) >=
    (current.discord_connection?.revision ?? 0)
      ? incoming.discord_connection
      : current.discord_connection;
  const voice_connection =
    (incoming.voice_connection?.revision ?? 0) >=
    (current.voice_connection?.revision ?? 0)
      ? incoming.voice_connection
      : current.voice_connection;
  return { ...engine, connections, discord_connection, voice_connection };
}

function App() {
  const [state, setState] = useState<State>();
  const [view, setView] = useState("people");
  const [theme, setTheme] = useState(() =>
    readLocal<string>("ladder-theme-v2", "dark") === "light" ? "light" : "dark",
  );
  const [adminID, setAdminID] = useState(() => {
    try {
      return sessionStorage.getItem("ladder-demo-admin") || "admin-maya";
    } catch {
      return "admin-maya";
    }
  });
  const [search, setSearch] = useState("");
  const [filter, setFilter] = useState("all");
  const [workspace, setWorkspace] = useState<"live" | "demo">("live");
  const [skill, setSkill] = useState("all");
  const [visible, setVisible] = useState(6);
  const [connected, setConnected] = useState(false);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [profilesOpen, setProfilesOpen] = useState(false);
  const [connectionsOpen, setConnectionsOpen] = useState(false);
  const [createOpen, setCreateOpen] = useState(false);
  const [voiceOpen, setVoiceOpen] = useState(false);
  const [detail, setDetail] = useState<Detail>();
  const [room, setRoom] = useState<string>();
  const [resetOpen, setResetOpen] = useState(false);
  const [activityOpen, setActivityOpen] = useState(false);
  const [discordOpen, setDiscordOpen] = useState(false);
  const [eventLimit, setEventLimit] = useState(30);
  const speech = useSpeech();
  function sync(s: State) {
    setState((current) => mergeState(current, s));
  }
  useEffect(() => {
    document.documentElement.dataset.theme = theme;
    try {
      localStorage.setItem("ladder-theme-v2", JSON.stringify(theme));
    } catch {}
  }, [theme]);
  useEffect(() => {
    try {
      sessionStorage.setItem("ladder-demo-admin", adminID);
    } catch {}
  }, [adminID]);
  useEffect(() => {
    const stream = new EventSource("/api/events");
    stream.onopen = () => setConnected(true);
    stream.onerror = () => setConnected(false);
    stream.onmessage = (e) => {
      try {
        sync(JSON.parse(e.data));
      } catch {
        setError("Could not read the workspace update.");
      }
    };
    return () => stream.close();
  }, []);
  useEffect(() => {
    function hash() {
      const match = location.hash.match(/^#room=(candidate-[a-z0-9-]+)$/);
      if (match) setRoom(match[1]);
    }
    hash();
    window.addEventListener("hashchange", hash);
    return () => window.removeEventListener("hashchange", hash);
  }, []);
  const action: Action = async (path, body = {}) => {
    if (busy) return false;
    setBusy(true);
    setError("");
    try {
      const response = await fetch(path, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify(body),
      });
      const next = await response.json();
      if (!response.ok)
        throw Error(next.error || "Could not save this change.");
      sync(next);
      return true;
    } catch (e) {
      setError(
        e instanceof Error ? e.message : "The local server is unavailable.",
      );
      return false;
    } finally {
      setBusy(false);
    }
  };
  const allCandidates = state?.candidates ?? [];
  const cs = allCandidates.filter(
    (c) => (c.mode === "live") === (workspace === "live"),
  );
  const ids = new Set(cs.map((c) => c.id));
  const requests = state?.requests.filter((r) => ids.has(r.candidate_id)) ?? [];
  const pending = requests.filter((r) => r.status === "pending");
  const events =
    state?.events.filter((e) =>
      e.candidate_id
        ? ids.has(e.candidate_id)
        : workspace === "demo" &&
          (e.kind === "progress" || e.kind === "control"),
    ) ?? [];
  const scopedState = state && { ...state, candidates: cs, requests, events };
  const active = cs.filter((c) =>
    c.mode === "live"
      ? c.live?.status === "generating" ||
        c.live?.status === "analyzing" ||
        c.live?.status === "awaiting_answer"
      : c.runs.some((r) => r.status === "active") &&
        c.stage !== "Review" &&
        !c.room.expired &&
        c.room.empty_since === null,
  );
  const spend = summarize(cs.flatMap((c) => c.live?.calls ?? []));
  const query = search.trim().toLowerCase();
  const hasFilters = query !== "" || skill !== "all" || filter !== "all";
  function clearFilters() {
    setSkill("all");
    setSearch("");
    setFilter("all");
    setVisible(6);
  }
  const filtered = cs.filter(
    (c) =>
      (skill === "all" || c.profile_id === skill) &&
      `${c.name} ${c.job} ${c.profile.name}`.toLowerCase().includes(query) &&
      (filter === "all" ||
        (filter === "active" && active.some((a) => a.id === c.id)) ||
        (filter === "human" && pending.some((r) => r.candidate_id === c.id)) ||
        (filter === "done" &&
          (c.stage === "Hired" || c.stage === "Rejected")) ||
        (filter === "failed" && interviewFailed(c))),
  );
  const selected = allCandidates.find((c) => c.id === detail?.id);
  const selectedRoom = allCandidates.find((c) => c.id === room);
  function open(id: string, tab = "Overview", agent = 0) {
    setRoom(undefined);
    setDetail({ id, tab, agent });
    if (allCandidates.find((c) => c.id === id)?.mode === "live")
      history.replaceState(null, "", `#room=${id}`);
  }
  async function openRoom(id: string) {
    if (allCandidates.find((c) => c.id === id)?.mode === "live") {
      open(id, "AI report");
      return;
    }
    setDetail(undefined);
    setRoom(id);
    history.replaceState(null, "", `#room=${id}`);
    const request = pending.find((r) => r.candidate_id === id);
    if (request)
      await action(`/api/requests/${request.id}`, {
        action: "open",
        admin_id: adminID,
      });
  }
  function closeDetail() {
    speech.stop();
    setDetail(undefined);
    setRoom(undefined);
    history.replaceState(null, "", location.pathname + location.search);
  }
  const liveTab = (t?: string) =>
    t === "AI report" ? "report" : t === "Agents" ? "team" : "conversation";
  function switchWorkspace(next: string) {
    setWorkspace(next === "demo" ? "demo" : "live");
    setFilter("all");
    setSkill("all");
    setSearch("");
    setVisible(6);
    setEventLimit(30);
  }
  function navigate(next: string) {
    setView(next);
    window.scrollTo({
      top: 0,
      behavior: window.matchMedia("(prefers-reduced-motion: reduce)").matches
        ? "instant"
        : "smooth",
    });
  }
  return (
    <div className="glass-shell">
      <aside className="floating-rail glass" aria-label="Workspace navigation">
        <a
          className="nav-brand"
          href="#"
          aria-label="Teamrise home"
          onClick={(e) => {
            e.preventDefault();
            navigate("people");
          }}
        >
          <SolMark />
        </a>
        <nav>
          {[
            { id: "people", label: "Candidate workspace", icon: LayoutGrid },
            { id: "review", label: "Human review queue", icon: Bell },
            {
              id: "insights",
              label: "Interview insights",
              icon: ChartNoAxesCombined,
            },
          ].map(({ id, label, icon: Icon }) => (
            <button
              key={id}
              className={`nav-icon ${view === id ? "selected" : ""}`}
              aria-label={label}
              title={label}
              aria-current={view === id ? "page" : undefined}
              onClick={() => navigate(id)}
            >
              <Icon size={20} />
              {id === "review" && pending.length > 0 && (
                <span className="nav-badge">{pending.length}</span>
              )}
            </button>
          ))}
          <span className="nav-separator" />
          <button
            className="nav-icon"
            aria-label="Interview skills and prompts"
            title="Interview skills"
            onClick={() => setProfilesOpen(true)}
          >
            <FileSliders size={20} />
          </button>
          <button
            className="nav-icon"
            aria-label="Connections and agents"
            title="Connections & agents"
            onClick={() => setConnectionsOpen(true)}
            disabled={!state}
          >
            <Settings2 size={20} />
          </button>
        </nav>
        {workspace === "demo" && (
          <span className="nav-person" title={reviewerNames[adminID]}>
            {reviewerNames[adminID]?.[0] ?? "M"}
          </span>
        )}
      </aside>
      <div className="workspace-canvas">
        <header className="floating-topbar glass">
          <div className="floating-brand">
            <span>Teamrise</span>
            <span className="header-divider" />
            <span className="header-context">
              {view === "people"
                ? "Workspace"
                : view === "review"
                  ? "Human review"
                  : "Insights"}
            </span>
          </div>
          <Tabs
            className="workspace-switch"
            label="Workspace mode"
            value={workspace}
            change={switchWorkspace}
            items={[
              { id: "live", label: "Live" },
              { id: "demo", label: "Demo" },
            ]}
          />
          <div className="header-tools">
            <span className="sync-status">
              <span className={`live-dot ${connected ? "live" : ""}`} />
              {connected ? "All changes saved" : "Connecting…"}
            </span>
            <button
              className="icon-button"
              aria-label="Voice settings"
              title="Voice settings"
              onClick={() => setVoiceOpen(true)}
            >
              <AudioLines size={18} />
            </button>
            <button
              className="icon-button"
              aria-label={
                theme === "dark"
                  ? "Switch to light theme"
                  : "Switch to dark theme"
              }
              title="Switch theme"
              onClick={() => setTheme(theme === "dark" ? "light" : "dark")}
            >
              {theme === "dark" ? <Sun size={18} /> : <Moon size={18} />}
            </button>
            {workspace === "live" && (
              <button
                className="button primary new-interview-button"
                disabled={!state}
                onClick={() => setCreateOpen(true)}
              >
                <Plus size={16} />
                <span>New interview</span>
              </button>
            )}
          </div>
        </header>
        <main className="workspace-main">
          {(!connected || error || speech.error) && (
            <div
              className={`workspace-notice ${error || speech.error ? "error" : ""}`}
              role="status"
            >
              <span>
                {error ||
                  speech.error ||
                  "Reconnecting to your saved workspace…"}
              </span>
              {error && (
                <button
                  className="icon-button"
                  aria-label="Dismiss error"
                  onClick={() => setError("")}
                >
                  <X size={15} />
                </button>
              )}
            </div>
          )}
          <div
            className="workspace-view tab-panel"
            key={`${workspace}-${view}`}
          >
            {view === "people" ? (
              <>
                <section className="workspace-intro">
                  <div>
                    <span className="eyebrow">
                      {workspace === "live"
                        ? "LIVE INTERVIEWS"
                        : "DEMO SANDBOX"}
                    </span>
                    <h1>
                      {workspace === "live"
                        ? "Interview workspace"
                        : "Practice workspace"}
                      <span>.</span>
                    </h1>
                    <p>
                      {workspace === "live"
                        ? "Follow interviews and review candidate evidence."
                        : "Explore the interview workflow with sample candidates."}
                    </p>
                  </div>
                  <div className="intro-actions">
                    <button
                      className="button quiet"
                      onClick={() => setProfilesOpen(true)}
                    >
                      <FileSliders size={15} />
                      Interview skills
                    </button>
                    <button
                      className="button quiet"
                      onClick={() => setActivityOpen(true)}
                    >
                      <Activity size={15} /> Event log
                    </button>
                  </div>
                </section>
                <section
                  className="compact-metrics"
                  aria-label="Workspace summary"
                >
                  <button
                    onClick={() => {
                      setFilter("all");
                    }}
                  >
                    <Users size={15} />
                    <strong>{cs.length}</strong>
                    <span>people</span>
                  </button>
                  <button onClick={() => setFilter("active")}>
                    <Activity size={15} />
                    <strong>{active.length}</strong>
                    <span>in progress</span>
                  </button>
                  <button
                    className={pending.length ? "has-requests" : ""}
                    onClick={() => navigate("review")}
                  >
                    <Bell size={15} />
                    <strong>{pending.length}</strong>
                    <span>need you</span>
                  </button>
                  {workspace === "live" && (
                    <button onClick={() => navigate("insights")}>
                      <Coins size={15} />
                      <strong>{spend.priced ? usd(spend.cost) : "—"}</strong>
                      <span>
                        AI spend{" "}
                        {spend.priced > 0 && spend.priced < spend.attempts
                          ? "· partial estimate"
                          : "· estimate"}
                      </span>
                    </button>
                  )}
                  {workspace === "live" && (
                    <span className="summary-model">
                      <span className="connection-dot" />
                      {state?.connections.ai.model || "AI model not configured"}
                    </span>
                  )}
                </section>
                {workspace === "demo" && state && (
                  <section className="demo-sandbox glass tab-panel">
                    <div>
                      <span className="eyebrow">SCRIPTED SANDBOX</span>
                      <h3>Try the journey without API calls.</h3>
                      <p>
                        {duration(state.clock_seconds)} elapsed ·{" "}
                        {state.settings.concurrent} concurrent demo rooms
                      </p>
                    </div>
                    <div className="sandbox-actions">
                      <button
                        className="button secondary"
                        disabled={busy || state.running}
                        onClick={() => action("/api/demo/step")}
                      >
                        <SkipForward size={14} />
                        Next tick
                      </button>
                      <button
                        className="button secondary"
                        disabled={busy}
                        onClick={() =>
                          action(
                            `/api/demo/${state.running ? "pause" : "start"}`,
                          )
                        }
                      >
                        {state.running ? (
                          <Pause size={14} />
                        ) : (
                          <Play size={14} />
                        )}
                        {state.running ? "Pause demo" : "Run demo"}
                      </button>
                      <button
                        className="icon-button"
                        disabled={busy}
                        aria-label="Reset demo"
                        title="Reset demo"
                        onClick={() => setResetOpen(true)}
                      >
                        <RotateCcw size={16} />
                      </button>
                      <button
                        className="icon-button"
                        aria-label="Open event log"
                        title="Event log"
                        onClick={() => setActivityOpen(true)}
                      >
                        <Activity size={16} />
                      </button>
                      <button
                        className="text-button"
                        onClick={() => setDiscordOpen(true)}
                      >
                        Discord preview
                      </button>
                    </div>
                  </section>
                )}
                <section
                  className="people-workspace"
                  aria-label="Candidate workspace"
                >
                  <div className="people-toolbar">
                    <Tabs
                      label="Candidate filters"
                      value={filter}
                      change={(t) => {
                        setFilter(t);
                        setVisible(6);
                      }}
                      items={[
                        { id: "all", label: "Everyone" },
                        { id: "active", label: "In progress" },
                        { id: "human", label: "Needs review" },
                        { id: "done", label: "Decided" },
                        {
                          id: "failed",
                          label: "Failed",
                          count: cs.filter(interviewFailed).length,
                        },
                      ]}
                    />
                    <div className="people-search-controls">
                      <label className="search-field">
                        <Search size={15} />
                        <input
                          aria-label="Search candidates"
                          placeholder="Find a person…"
                          value={search}
                          onChange={(e) => {
                            setSearch(e.target.value);
                            setVisible(6);
                          }}
                        />
                      </label>
                      <GlassSelect
                        label="Filter by interview skill"
                        value={skill}
                        onChange={(value) => {
                          setSkill(value);
                          setVisible(6);
                        }}
                        options={[
                          { value: "all", label: "All skills" },
                          ...(state?.profiles.map((p) => ({
                            value: p.id,
                            label: p.name,
                          })) ?? []),
                        ]}
                      />
                    </div>
                  </div>
                  <div className="people-section-caption">
                    <span role="status">
                      {filtered.length}{" "}
                      {filtered.length === 1 ? "person" : "people"} in this view
                    </span>
                    {hasFilters && filtered.length > 0 && (
                      <button
                        className="text-button clear-candidate-filters"
                        onClick={clearFilters}
                      >
                        <X size={13} aria-hidden="true" />
                        Clear filters
                      </button>
                    )}
                  </div>
                  <div className="people-grid">
                    {state &&
                      filtered
                        .slice(0, visible)
                        .map((c) => (
                          <PersonCard
                            key={c.id}
                            candidate={c}
                            state={state}
                            busy={busy}
                            action={action}
                            open={open}
                            room={openRoom}
                          />
                        ))}
                  </div>
                  {!filtered.length && (
                    <div className="quiet-empty glass">
                      <Users size={35} />
                      <h2>
                        {!state
                          ? "Opening your workspace…"
                          : hasFilters
                            ? "No matching candidates"
                            : workspace === "live"
                              ? "No live interviews yet"
                              : "No demo candidates yet"}
                      </h2>
                      <p>
                        {!state
                          ? "Your saved records will appear shortly."
                          : hasFilters
                            ? "Try a different name or clear the filters to see everyone."
                            : workspace === "live"
                              ? "Start an interview to see the conversation and review evidence here."
                              : "Start the demo to see candidates move through the interview stages."}
                      </p>
                      {state && (hasFilters || workspace === "live") && (
                        <div className="form-actions">
                          {hasFilters ? (
                            <button
                              className="button secondary"
                              onClick={clearFilters}
                            >
                              Clear filters
                            </button>
                          ) : (
                            <button
                              className="button primary"
                              onClick={() => setCreateOpen(true)}
                            >
                              <Plus size={15} />
                              New interview
                            </button>
                          )}
                        </div>
                      )}
                    </div>
                  )}
                  {filtered.length > visible && (
                    <div className="load-more">
                      <span>
                        Showing {Math.min(visible, filtered.length)} of{" "}
                        {filtered.length} people
                      </span>
                      <button
                        className="button secondary"
                        onClick={() => setVisible(visible + 6)}
                      >
                        Show more people
                        <ArrowRight size={14} />
                      </button>
                    </div>
                  )}
                </section>
              </>
            ) : view === "review" ? (
              scopedState ? (
                <ReviewQueue
                  state={scopedState}
                  adminID={adminID}
                  setAdminID={setAdminID}
                  open={openRoom}
                />
              ) : (
                <div className="quiet-empty">Opening the shared queue…</div>
              )
            ) : scopedState ? (
              <Insights state={scopedState} demo={workspace === "demo"} />
            ) : (
              <div className="quiet-empty">Loading interview insights…</div>
            )}
          </div>
          <footer className="canvas-footer">
            <span>
              Teamrise <i />A little more human.
            </span>
            <span>
              v0.9.3 ·{" "}
              {workspace === "demo"
                ? "Demo workspace"
                : state?.voice_connection?.ready
                  ? "Discord voice ready"
                  : state?.discord_connection?.connected
                    ? "Discord connected"
                    : "Discord offline"}
            </span>
            <button
              className="text-button"
              onClick={() => setConnectionsOpen(true)}
            >
              Connections <ArrowRight size={12} />
            </button>
          </footer>
        </main>
      </div>
      {profilesOpen && state && (
        <SkillEditor
          state={state}
          action={action}
          busy={busy}
          close={() => setProfilesOpen(false)}
        />
      )}
      {connectionsOpen && state && (
        <ConnectionsPanel
          state={state}
          action={action}
          close={() => setConnectionsOpen(false)}
        />
      )}
      {createOpen && state && (
        <CreateInterview
          state={state}
          close={() => setCreateOpen(false)}
          created={(s, id) => {
            sync(s);
            setCreateOpen(false);
            setView("people");
            setWorkspace("live");
            setFilter("all");
            setSkill("all");
            setSearch("");
            open(id);
          }}
        />
      )}
      {voiceOpen && (
        <VoiceSettings
          speech={speech}
          close={() => {
            speech.stop();
            setVoiceOpen(false);
          }}
        />
      )}
      {selected &&
        state &&
        (selected.mode === "live" ? (
          <LiveInterview
            key={selected.id}
            candidate={selected}
            state={state}
            speech={speech}
            adminID={adminID}
            action={action}
            sync={sync}
            initialTab={liveTab(detail?.tab)}
            close={closeDetail}
          />
        ) : (
          <CandidateInspector
            key={selected.id}
            candidate={selected}
            state={state}
            speech={speech}
            initialTab={detail?.tab}
            initialAgent={detail?.agent}
            close={closeDetail}
          />
        ))}
      {selectedRoom &&
        state &&
        (selectedRoom.mode === "live" ? (
          <LiveInterview
            key={selectedRoom.id}
            candidate={selectedRoom}
            state={state}
            speech={speech}
            adminID={adminID}
            action={action}
            sync={sync}
            close={closeDetail}
          />
        ) : (
          <RoomMonitor
            candidate={selectedRoom}
            state={state}
            adminID={adminID}
            action={action}
            busy={busy}
            close={closeDetail}
          />
        ))}
      {resetOpen && (
        <Modal
          title="A fresh demo run?"
          close={() => setResetOpen(false)}
          footer={
            <div className="form-actions">
              <button
                className="button secondary"
                onClick={() => setResetOpen(false)}
              >
                Keep this run
              </button>
              <button
                className="button primary"
                disabled={busy}
                onClick={async () => {
                  if (await action("/api/demo/reset")) {
                    speech.stop();
                    setResetOpen(false);
                  }
                }}
              >
                Reset demo
              </button>
            </div>
          }
        >
          <div className="modal-body">
            <p className="modal-description">
              Demo journeys and rooms restart. Real interviews, their review
              requests and your saved skills stay intact.
            </p>
          </div>
        </Modal>
      )}
      {activityOpen && state && (
        <Modal
          title="Workspace event log"
          close={() => setActivityOpen(false)}
          wide
          toolbar={
            <div className="modal-log-toolbar">
              <p className="soft-note">
                {events.length} interview events in this workspace.
              </p>
              <a
                href={`/api/export?workspace=${workspace}`}
                className="button secondary"
                download
              >
                <ArrowDownToLine size={14} />
                Export JSON
              </a>
            </div>
          }
        >
          <div className="modal-body">
            <div className="event-list">
              {[...events]
                .reverse()
                .slice(0, eventLimit)
                .map((e) => (
                  <article key={e.id} className="system-event">
                    <span className="event-avatar">
                      <Check size={16} />
                    </span>
                    <div>
                      <strong>
                        {e.agent}
                        {e.candidate_name ? ` · ${e.candidate_name}` : ""}
                      </strong>
                      <p>{e.text}</p>
                    </div>
                    <time>
                      {new Date(e.time).toLocaleTimeString([], {
                        hour: "2-digit",
                        minute: "2-digit",
                      })}
                    </time>
                  </article>
                ))}
            </div>
            {events.length > eventLimit && (
              <button
                className="button secondary"
                onClick={() => setEventLimit(eventLimit + 30)}
              >
                Show older events
              </button>
            )}
          </div>
        </Modal>
      )}
      {discordOpen && (
        <Modal
          title="Scripted Discord preview"
          close={() => setDiscordOpen(false)}
          className="discord-preview-dialog"
          wide
        >
          <div className="modal-body">
            <p className="soft-note">
              Demo message components only. No Discord connection is active.
            </p>
            <Suspense fallback={<p>Loading preview…</p>}>
              <DiscordPreview state={scopedState} />
            </Suspense>
          </div>
        </Modal>
      )}
      {speech.playing && (
        <div className="speech-player glass" role="status">
          <AudioLines size={18} />
          <span>
            {speech.paused ? "Playback paused" : "Reading the conversation"}
          </span>
          <button
            className="icon-button"
            aria-label={speech.paused ? "Resume playback" : "Pause playback"}
            onClick={speech.togglePause}
          >
            {speech.paused ? <Play size={16} /> : <Pause size={16} />}
          </button>
          <button
            className="icon-button"
            aria-label="Stop playback"
            onClick={speech.stop}
          >
            <Square size={15} />
          </button>
        </div>
      )}
    </div>
  );
}
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <App />
  </React.StrictMode>,
);
