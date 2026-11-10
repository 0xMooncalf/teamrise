import GlassSelect from "./GlassSelect";
import { useState } from "react";
import { ArrowUpRight, Check, Clock3, ShieldCheck, Users } from "lucide-react";
import Tabs from "./Tabs";
import { Avatar, duration } from "./shared";
import type { State } from "./types";

export const reviewerNames: Record<string, string> = {
  "admin-maya": "Maya Chen",
  "admin-theo": "Theo Brooks",
  "admin-jules": "Jules Rivera",
};
export default function ReviewQueue({
  state,
  adminID,
  setAdminID,
  open,
}: {
  state: State;
  adminID: string;
  setAdminID: (id: string) => void;
  open: (id: string) => void;
}) {
  const [tab, setTab] = useState("pending");
  const [limit, setLimit] = useState(9);
  const waiting = state.requests.filter((r) => r.status === "pending");
  const history = state.requests.filter((r) => r.status !== "pending");
  const list = tab === "pending" ? waiting : history;
  return (
    <section className="review-workspace" aria-label="Shared admin dashboard">
      <div className="section-heading">
        <div>
          <span className="eyebrow">THE HUMAN PART</span>
          <h2>A good decision deserves a person.</h2>
          <p>
            One shared queue. Claims and confirmed joins update every dashboard.
          </p>
        </div>
        {state.candidates.some((c) => !c.live?.voice) && (
          <label className="reviewer-select">
            <span>Reviewing as</span>
            <GlassSelect
              label="Local reviewer identity"
              value={adminID}
              onChange={setAdminID}
              options={Object.entries(reviewerNames).map(([value, label]) => ({
                value,
                label,
              }))}
            />
          </label>
        )}
      </div>
      <Tabs
        label="Review queue sections"
        value={tab}
        change={(t) => {
          setTab(t);
          setLimit(9);
        }}
        items={[
          { id: "pending", label: "Needs a human", count: waiting.length },
          { id: "history", label: "History", count: history.length },
        ]}
      />
      <div className="tab-panel" key={tab} role="tabpanel">
        {list.length ? (
          <div className="review-grid">
            {[...list]
              .reverse()
              .slice(0, limit)
              .map((r) => {
                const c = state.candidates.find((c) => c.id === r.candidate_id);
                const live = c?.mode === "live";
                const voice = c?.live?.voice;
                return (
                  <article
                    className={`review-card glass ${r.status !== "pending" ? "handled" : ""}`}
                    key={r.id}
                    aria-label={`${r.candidate_name}: ${r.status}`}
                  >
                    <div className="request-person">
                      {c && <Avatar candidate={c} />}
                      <div>
                        <strong>{r.candidate_name}</strong>
                        <span>
                          {c?.profile.name} · {live ? "Real AI" : "Demo"}
                        </span>
                      </div>
                      {r.status !== "pending" ? (
                        <Check size={18} />
                      ) : (
                        <span className="waiting-dot" />
                      )}
                    </div>
                    <h3>{r.reason}</h3>
                    <p>
                      {r.status === "pending"
                        ? voice
                          ? c.room.expired
                            ? "The room closed. The saved transcript is available for review."
                            : "Join the private Discord room with the Reviewer role. Discord confirms attendance for everyone."
                          : live
                            ? "The report is ready. Review the evidence and claim the next step."
                            : "Join the simulated room to record a human decision."
                        : r.attended_by
                          ? `${r.attended_by} ${live && !voice ? "claimed this review." : "joined the room."}`
                          : "This request was closed."}
                    </p>
                    <div className="review-card-meta">
                      <Clock3 size={12} />
                      {r.status === "pending"
                        ? `${duration(r.created_timestamp ? Math.max(0, (Date.now() - Date.parse(r.created_timestamp)) / 1000) : state.clock_seconds - r.created_at)} waiting`
                        : r.status}
                    </div>
                    {voice &&
                      r.status === "pending" &&
                      c?.live?.discord?.channel_id &&
                      !c.room.expired && (
                        <a
                          className="button primary"
                          href={`https://discord.com/channels/${voice.guild_id}/${c.live.discord.channel_id}`}
                          target="_blank"
                          rel="noreferrer"
                        >
                          Join Discord room <ArrowUpRight size={14} />
                        </a>
                      )}
                    <button
                      className={`button ${r.status === "pending" ? "primary" : "secondary"}`}
                      disabled={r.status !== "pending"}
                      onClick={() => open(r.candidate_id)}
                    >
                      {r.status === "pending"
                        ? live
                          ? "Review interview"
                          : "Open demo room"
                        : live
                          ? "Review claimed"
                          : "Entry confirmed"}
                      <ArrowUpRight size={14} />
                    </button>
                  </article>
                );
              })}
          </div>
        ) : (
          <div className="quiet-empty glass">
            <Users size={34} />
            <h3>
              {tab === "pending"
                ? "No one is waiting on you."
                : "A clear record of every handoff."}
            </h3>
            <p>
              {tab === "pending"
                ? "You’ll see a request when a report needs your judgment."
                : "Claimed reviews and confirmed demo-room entries appear here."}
            </p>
          </div>
        )}
        {list.length > limit && (
          <div className="load-more">
            <button
              className="button secondary"
              onClick={() => setLimit(limit + 9)}
            >
              Show more requests
            </button>
          </div>
        )}
      </div>
      <p className="fine-print">
        <ShieldCheck size={13} />
        Local reviewer names are testing identities for web/demo reviews. Voice
        attendance uses actual Discord membership and the configured Reviewer
        role.
      </p>
    </section>
  );
}
