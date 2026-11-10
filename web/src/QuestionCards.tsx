import {
  useEffect,
  useRef,
  useState,
  type Dispatch,
  type PointerEvent,
  type SetStateAction,
} from "react";
import {
  ArrowDown,
  ArrowUp,
  GripVertical,
  ImagePlus,
  MessageSquare,
  Mic,
  Plus,
  Trash2,
  X,
} from "lucide-react";
import GlassSelect from "./GlassSelect";
import type { InterviewCard, InterviewProfile } from "./types";

export function cardsFor(profile: InterviewProfile): InterviewCard[] {
  return (
    profile.cards?.map((card) => ({ ...card })) ??
    profile.questions.map((prompt) => ({
      id: "card-" + crypto.randomUUID(),
      channel: "voice",
      prompt,
    }))
  );
}

type Props = {
  cards: InterviewCard[];
  setCards: Dispatch<SetStateAction<InterviewCard[]>>;
  onUploadChange: (delta: number) => void;
};

type CardDrag = {
  id: string;
  pointer: number;
  x: number;
  y: number;
  startY: number;
  stack: Element;
  scroll: HTMLElement | null;
  frame: number;
};

export default function QuestionCards({
  cards,
  setCards,
  onUploadChange,
}: Props) {
  const [dragged, setDragged] = useState<string>();
  const [target, setTarget] = useState<string>();
  const drag = useRef<CardDrag | null>(null);
  useEffect(() => () => cancelAnimationFrame(drag.current?.frame ?? 0), []);

  function targetAt(current: CardDrag) {
    const card = document
      .elementFromPoint(current.x, current.y)
      ?.closest<HTMLElement>(".question-card");
    return card && current.stack.contains(card) ? card.dataset.card : undefined;
  }
  function startDrag(e: PointerEvent<HTMLElement>, id: string) {
    const stack = e.currentTarget.closest(".question-card-stack");
    if (e.button !== 0 || !stack || drag.current) return;
    e.preventDefault();
    e.currentTarget.setPointerCapture(e.pointerId);
    const current: CardDrag = {
      id,
      pointer: e.pointerId,
      x: e.clientX,
      y: e.clientY,
      startY: e.clientY,
      stack,
      scroll: stack.closest<HTMLElement>(".modal-scroll"),
      frame: 0,
    };
    drag.current = current;
    setDragged(id);
    function track() {
      if (drag.current !== current) return;
      if (Math.abs(current.y - current.startY) > 4) {
        const scroll = current.scroll;
        if (scroll) {
          const bounds = scroll.getBoundingClientRect();
          if (current.y > bounds.bottom - 34 && current.y < bounds.bottom + 12)
            scroll.scrollTop += 9;
          else if (current.y < bounds.top + 34 && current.y > bounds.top - 12)
            scroll.scrollTop -= 9;
        }
        setTarget(targetAt(current));
      }
      current.frame = requestAnimationFrame(track);
    }
    current.frame = requestAnimationFrame(track);
  }
  function finishDrag(e: PointerEvent<HTMLElement>, drop: boolean) {
    const current = drag.current;
    if (!current || e.pointerId !== current.pointer) return;
    drag.current = null;
    cancelAnimationFrame(current.frame);
    if (drop && Math.abs(e.clientY - current.startY) > 4) {
      current.x = e.clientX;
      current.y = e.clientY;
      const id = targetAt(current);
      move(
        current.id,
        cards.findIndex((card) => card.id === id),
      );
    }
    setDragged(undefined);
    setTarget(undefined);
  }

  function move(id: string, to: number) {
    setCards((list) => {
      const from = list.findIndex((card) => card.id === id);
      if (from < 0 || to < 0 || to >= list.length || from === to) return list;
      const next = [...list];
      const [card] = next.splice(from, 1);
      next.splice(to, 0, card);
      return next;
    });
  }
  function update(id: string, fields: Partial<InterviewCard>) {
    setCards((list) =>
      list.map((card) => (card.id === id ? { ...card, ...fields } : card)),
    );
  }
  function add(channel: InterviewCard["channel"]) {
    setCards((list) =>
      list.length >= 8
        ? list
        : [
            ...list,
            {
              id: "card-" + crypto.randomUUID(),
              channel,
              prompt: "",
            },
          ],
    );
  }

  return (
    <section
      className="question-card-editor"
      aria-label="Interview question cards"
    >
      <div className="question-card-toolbar">
        <div>
          <h3>Question cards</h3>
          <p>
            Drag to reorder, or use the arrow buttons. The interviewer follows
            this order.
          </p>
        </div>
        <span className="status-badge">{cards.length} / 8</span>
      </div>
      <div className="question-card-stack">
        {cards.map((card, index) => (
          <article
            key={card.id}
            data-card={card.id}
            className={
              target === card.id && dragged !== card.id
                ? "question-card drop-target"
                : dragged === card.id
                  ? "question-card dragging"
                  : "question-card"
            }
          >
            <div className="question-card-head">
              <span
                className="question-card-grip"
                title="Drag to reorder"
                onPointerDown={(e) => startDrag(e, card.id)}
                onPointerMove={(e) => {
                  if (drag.current?.pointer !== e.pointerId) return;
                  drag.current.x = e.clientX;
                  drag.current.y = e.clientY;
                }}
                onPointerUp={(e) => finishDrag(e, true)}
                onPointerCancel={(e) => finishDrag(e, false)}
                onLostPointerCapture={(e) => finishDrag(e, false)}
              >
                <GripVertical size={18} aria-hidden="true" />
              </span>
              <strong>Question {index + 1}</strong>
              <div className="question-card-controls">
                <button
                  type="button"
                  className="icon-button"
                  aria-label={"Move question " + (index + 1) + " up"}
                  disabled={index === 0}
                  onClick={() => move(card.id, index - 1)}
                >
                  <ArrowUp size={15} />
                </button>
                <button
                  type="button"
                  className="icon-button"
                  aria-label={"Move question " + (index + 1) + " down"}
                  disabled={index === cards.length - 1}
                  onClick={() => move(card.id, index + 1)}
                >
                  <ArrowDown size={15} />
                </button>
                <button
                  type="button"
                  className="icon-button remove-card"
                  aria-label={"Remove question " + (index + 1)}
                  disabled={cards.length === 1}
                  onClick={() =>
                    setCards((list) => list.filter((q) => q.id !== card.id))
                  }
                >
                  <Trash2 size={15} />
                </button>
              </div>
            </div>
            <div className="question-card-format">
              {card.channel === "chat" ? (
                <MessageSquare size={16} />
              ) : (
                <Mic size={16} />
              )}
              <GlassSelect
                label={"Format for question " + (index + 1)}
                value={card.channel}
                options={[
                  { value: "voice", label: "Spoken question" },
                  { value: "chat", label: "Practical · Discord chat" },
                ]}
                onChange={(channel) =>
                  update(card.id, {
                    channel: channel === "chat" ? "chat" : "voice",
                  })
                }
              />
            </div>
            <label className="form-field">
              <span>Question {index + 1} prompt</span>
              <textarea
                rows={3}
                maxLength={600}
                placeholder={
                  card.channel === "chat"
                    ? "Explain what you see or solve the exercise…"
                    : "Ask a question…"
                }
                value={card.prompt}
                onChange={(e) => update(card.id, { prompt: e.target.value })}
              />
              <small>
                {card.channel === "chat"
                  ? "Posted in the private room chat with the optional screenshot."
                  : "Read aloud in the voice room. The candidate answers by speaking."}
              </small>
            </label>
            {card.channel === "chat" && (
              <ScreenshotField
                image={card.image}
                onChange={(image) => update(card.id, { image })}
                onUploadChange={onUploadChange}
              />
            )}
            <details className="question-card-notes">
              <summary>
                Assessment notes {card.notes?.trim() ? "· added" : ""}
              </summary>
              <label className="form-field">
                <span>Expected evidence or screenshot context</span>
                <textarea
                  rows={2}
                  maxLength={1000}
                  value={card.notes ?? ""}
                  onChange={(e) => update(card.id, { notes: e.target.value })}
                  placeholder="Describe what a good answer should notice or explain."
                />
                <small>
                  Used by the AI for follow-ups and assessment. These notes stay
                  out of the Discord message.
                </small>
              </label>
            </details>
          </article>
        ))}
      </div>
      <div className="question-card-add">
        <button
          type="button"
          className="button secondary"
          disabled={cards.length >= 8}
          onClick={() => add("voice")}
        >
          <Plus size={15} /> Add spoken question
        </button>
        <button
          type="button"
          className="button secondary"
          disabled={cards.length >= 8}
          onClick={() => add("chat")}
        >
          <Plus size={15} /> Add practical task
        </button>
      </div>
    </section>
  );
}

function ScreenshotField({
  image,
  onChange,
  onUploadChange,
}: {
  image?: string;
  onChange: (id?: string) => void;
  onUploadChange: (delta: number) => void;
}) {
  const input = useRef<HTMLInputElement>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");

  async function upload(file: File) {
    setError("");
    if (file.size > 4 * 1024 * 1024) {
      setError("Choose a PNG or JPEG up to 4 MB.");
      return;
    }
    setBusy(true);
    onUploadChange(1);
    try {
      const response = await fetch("/api/screenshots", {
        method: "POST",
        headers: { "Content-Type": "application/octet-stream" },
        body: file,
      });
      const data = await response.json();
      if (!response.ok || !data.id)
        throw new Error(data.error || "Could not upload the screenshot.");
      onChange(data.id);
    } catch (err) {
      setError(
        err instanceof Error ? err.message : "Could not upload the screenshot.",
      );
    } finally {
      setBusy(false);
      onUploadChange(-1);
    }
  }

  return (
    <div className="question-screenshot">
      {image && (
        <img
          src={"/api/screenshots/" + image}
          alt="Practical exercise screenshot"
        />
      )}
      <div>
        <input
          ref={input}
          type="file"
          hidden
          accept="image/png,image/jpeg"
          onChange={(e) => {
            const file = e.target.files?.[0];
            e.target.value = "";
            if (file) void upload(file);
          }}
        />
        <button
          type="button"
          className="button secondary"
          disabled={busy}
          onClick={() => input.current?.click()}
        >
          <ImagePlus size={15} />{" "}
          {busy
            ? "Uploading…"
            : image
              ? "Replace screenshot"
              : "Upload screenshot"}
        </button>
        {image && (
          <button
            type="button"
            className="icon-button"
            aria-label="Remove screenshot"
            disabled={busy}
            onClick={() => onChange(undefined)}
          >
            <X size={15} />
          </button>
        )}
        <small>PNG or JPEG · up to 4 MB</small>
      </div>
      {error && (
        <p role="alert" className="connection-result failed">
          {error}
        </p>
      )}
    </div>
  );
}
