import GlassSelect from "./GlassSelect";
import { useState } from "react";
import { Plus, Save, WandSparkles } from "lucide-react";
import { Modal } from "./shared";
import Tabs from "./Tabs";
import QuestionCards, { cardsFor } from "./QuestionCards";
import type { Action, InterviewProfile, State } from "./types";

export default function SkillEditor({
  state,
  action,
  busy,
  close,
}: {
  state: State;
  action: Action;
  busy: boolean;
  close: () => void;
}) {
  const [tab, setTab] = useState("prompt");
  const [draft, setDraft] = useState<InterviewProfile>(() =>
    structuredClone(state.profiles[0]),
  );
  const [cards, setDraftCards] = useState(() => cardsFor(draft));
  const [cardsEdited, setCardsEdited] = useState(!!draft.cards?.length);
  const [uploads, setUploads] = useState(0);
  const setCards: typeof setDraftCards = (update) => {
    setCardsEdited(true);
    setDraftCards(update);
  };
  const [criteria, setCriteria] = useState(draft.criteria.join("\n"));
  const [message, setMessage] = useState("");
  const [concurrent, setConcurrent] = useState(state.settings.concurrent);
  const [threshold, setThreshold] = useState(state.settings.handoff_score);
  function load(p: InterviewProfile) {
    setDraft(structuredClone(p));
    setDraftCards(cardsFor(p));
    setCardsEdited(!!p.cards?.length);
    setCriteria(p.criteria.join("\n"));
    setMessage("");
  }
  async function save() {
    const q = cards.map((card) => card.prompt.trim());
    const cr = criteria
      .split("\n")
      .map((s) => s.trim())
      .filter(Boolean);
    if (
      draft.name.trim().length < 2 ||
      draft.skill_prompt.trim().length < 20 ||
      q.length < 1 ||
      q.length > 8 ||
      q.some((prompt) => prompt.length < 3) ||
      cr.length < 1 ||
      cr.length > 6
    ) {
      setMessage(
        "Use a role name, a prompt of at least 20 characters, 1–8 questions and 1–6 criteria.",
      );
      return;
    }
    setMessage(
      (await action("/api/profiles", {
        ...draft,
        questions: q,
        criteria: cr,
        cards: cardsEdited
          ? cards.map((card) =>
              card.channel === "voice" ? { ...card, image: undefined } : card,
            )
          : undefined,
      }))
        ? "Skill saved. Existing interviews keep their pinned version."
        : "Could not save this skill. Check the role, prompt and question lengths.",
    );
  }
  function suggest() {
    const text = (draft.name + " " + draft.skill_prompt).toLowerCase();
    const id = /frontend|react|css|accessib|фронт/.test(text)
      ? "frontend"
      : /clean|janitor|убор|чист/.test(text)
        ? "cleaning"
        : /backend|golang|database|бэкенд/.test(text)
          ? "backend"
          : "community";
    const preset = state.profiles.find((p) => p.id === id);
    if (preset) {
      setDraft({ ...draft, track: preset.track });
      setCards(cardsFor(preset));
      setCriteria(preset.criteria.join("\n"));
      setMessage("Loaded an editable demo preset. No AI request was made.");
    }
  }
  return (
    <Modal
      title="Interview skills & prompts"
      close={close}
      wide
      scrollKey={tab}
      toolbar={
        <>
          <div className="skill-select-row">
            <label className="form-field">
              <span>Skill profile</span>
              <GlassSelect
                label="Skill profile"
                value={draft.id}
                onChange={(value) => {
                  const profile = state.profiles.find((p) => p.id === value);
                  if (profile) load(profile);
                }}
                options={[
                  ...(!state.profiles.some((p) => p.id === draft.id)
                    ? [{ value: draft.id, label: "New custom skill" }]
                    : []),
                  ...state.profiles.map((p) => ({
                    value: p.id,
                    label: p.name,
                  })),
                ]}
              />
            </label>
            <button
              className="button secondary"
              onClick={() => {
                load({
                  id: `custom-${Date.now().toString(36)}`,
                  name: "Custom role",
                  track: "general",
                  skill_prompt: "",
                  questions: [
                    "Describe a typical task in this role and how you complete it.",
                  ],
                  criteria: ["Practical approach", "Communication"],
                });
                setTab("prompt");
              }}
            >
              <Plus size={14} />
              New skill
            </button>
          </div>
          <Tabs
            label="Skill editor sections"
            value={tab}
            change={setTab}
            items={[
              { id: "prompt", label: "Skill & prompt" },
              { id: "questions", label: "Questions & criteria" },
              { id: "workflow", label: "Workflow" },
            ]}
          />
        </>
      }
      footer={
        tab !== "workflow" && (
          <footer className="connection-footer skill-footer">
            {message && (
              <p className="skill-save-message" role="status">
                {message}
              </p>
            )}
            <button
              className="text-button"
              disabled={busy || !state.profiles.some((p) => p.id === draft.id)}
              onClick={async () =>
                setMessage(
                  (await action("/api/candidates/queued/profile", {
                    profile_id: draft.id,
                  }))
                    ? "Saved skill applied to queued demo candidates."
                    : "No queued candidates could be updated.",
                )
              }
            >
              Apply saved skill to queued demo candidates
            </button>
            <button
              className="button primary"
              disabled={busy || uploads > 0}
              onClick={save}
            >
              <Save size={14} />
              Save skill
            </button>
          </footer>
        )
      }
    >
      <div className="skill-editor-content tab-panel" role="tabpanel" key={tab}>
        {tab === "prompt" ? (
          <>
            <p className="soft-note">
              Give Echo a specialty and a clear brief. The same structure works
              for frontend, backend, cleaning or any other role.
            </p>
            <label className="form-field">
              <span>Role name</span>
              <input
                value={draft.name}
                maxLength={100}
                onChange={(e) => setDraft({ ...draft, name: e.target.value })}
              />
            </label>
            <label className="form-field">
              <span>Specialist skill prompt</span>
              <textarea
                rows={7}
                value={draft.skill_prompt}
                maxLength={8000}
                onChange={(e) =>
                  setDraft({ ...draft, skill_prompt: e.target.value })
                }
                placeholder="You are interviewing for… Ask about… Assess… Escalate when…"
              />
              <small>
                {draft.skill_prompt.length}/8000 characters · saved with each
                new interview
              </small>
            </label>
          </>
        ) : tab === "questions" ? (
          <>
            <QuestionCards
              cards={cards}
              setCards={setCards}
              onUploadChange={(delta) => setUploads((count) => count + delta)}
            />
            <label className="form-field">
              <span>Assessment criteria</span>
              <textarea
                rows={3}
                value={criteria}
                onChange={(e) => setCriteria(e.target.value)}
              />
              <small>
                1–6 criteria, one per line. Each scored criterion requires a
                quote from the candidate.
              </small>
            </label>
            <div className="suggestion-row">
              <button className="button secondary" onClick={suggest}>
                <WandSparkles size={14} />
                Suggest demo questions
              </button>
              <span>Editable presets · no model call</span>
            </div>
            <label className="form-field">
              <span>Demo answer scenario</span>
              <GlassSelect
                label="Demo answer scenario"
                value={draft.track}
                onChange={(value) => setDraft({ ...draft, track: value })}
                options={[
                  { value: "backend", label: "Backend" },
                  { value: "frontend", label: "Frontend" },
                  { value: "cleaning", label: "Cleaning" },
                  { value: "general", label: "General" },
                ]}
              />
            </label>
          </>
        ) : (
          <>
            <span className="eyebrow">WORKSPACE RULES</span>
            <h3>Keep the handoff deliberate.</h3>
            <div className="connection-fields">
              <label className="form-field">
                <span>Concurrent demo journeys</span>
                <input
                  type="number"
                  min={1}
                  max={8}
                  value={concurrent}
                  onChange={(e) => setConcurrent(Number(e.target.value))}
                />
                <small>
                  Controls scripted demo journeys, not real API or Discord voice
                  capacity.
                </small>
              </label>
              <label className="form-field">
                <span>Score for final human review</span>
                <input
                  type="number"
                  min={0}
                  max={100}
                  value={threshold}
                  onChange={(e) => setThreshold(Number(e.target.value))}
                />
                <small>
                  Lower scores request evidence review. Every final decision
                  remains human.
                </small>
              </label>
            </div>
            <button
              className="button primary"
              disabled={busy}
              onClick={async () =>
                setMessage(
                  (await action("/api/settings", {
                    concurrent,
                    handoff_score: threshold,
                  }))
                    ? "Workflow settings saved."
                    : "Could not save workflow settings.",
                )
              }
            >
              Save workflow
            </button>
            <p className="soft-note">
              Real interviews can currently call the provider in parallel. A
              global budget and rate-limit queue are future production work.
            </p>
          </>
        )}
        {tab === "workflow" && message && (
          <p className="inline-notice" role="status">
            {message}
          </p>
        )}
      </div>
    </Modal>
  );
}
