import { useId, useLayoutEffect, useRef, useState } from "react";
import { Bot, Check, Circle, CircleAlert, X } from "lucide-react";
import type { Candidate } from "./types";

export function duration(seconds: number) {
  const s = Math.max(0, Math.floor(seconds));
  return s >= 3600
    ? `${Math.floor(s / 3600)}h ${Math.floor((s % 3600) / 60)}m`
    : `${Math.floor(s / 60)}m ${String(s % 60).padStart(2, "0")}s`;
}
export function countdown(seconds: number) {
  const s = Math.max(0, Math.ceil(seconds));
  return `${String(Math.floor(s / 60)).padStart(2, "0")}:${String(s % 60).padStart(2, "0")}`;
}
export function Modal({
  title,
  close,
  children,
  wide = false,
  className = "",
  toolbar,
  footer,
  scrollKey,
}: {
  title: string;
  close: () => void;
  children: React.ReactNode;
  wide?: boolean;
  className?: string;
  toolbar?: React.ReactNode;
  footer?: React.ReactNode;
  scrollKey?: string;
}) {
  const titleID = useId();
  const ref = useRef<HTMLDialogElement>(null);
  const [closing, setClosing] = useState(false);
  const closeTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backdropPress = useRef(false);
  function isBackdrop(
    e:
      | React.PointerEvent<HTMLDialogElement>
      | React.MouseEvent<HTMLDialogElement>,
  ) {
    if (e.target !== e.currentTarget) return false;
    const bounds = e.currentTarget.getBoundingClientRect();
    return (
      e.clientX < bounds.left ||
      e.clientX > bounds.right ||
      e.clientY < bounds.top ||
      e.clientY > bounds.bottom
    );
  }
  function requestClose() {
    if (closing) return;
    if (window.matchMedia("(prefers-reduced-motion: reduce)").matches) {
      close();
      return;
    }
    setClosing(true);
    closeTimer.current = setTimeout(close, 150);
  }
  useLayoutEffect(() => {
    const dialog = ref.current;
    dialog?.showModal();
    return () => {
      if (closeTimer.current) clearTimeout(closeTimer.current);
      dialog?.close();
    };
  }, []);
  return (
    <dialog
      className={`modal ${wide ? "wide" : ""} ${closing ? "closing" : ""} ${className}`}
      ref={ref}
      aria-labelledby={titleID}
      onCancel={(e) => {
        e.preventDefault();
        requestClose();
      }}
      onPointerDown={(e) => {
        backdropPress.current = isBackdrop(e);
      }}
      onClick={(e) => {
        if (backdropPress.current && isBackdrop(e)) requestClose();
        backdropPress.current = false;
      }}
    >
      <header className="modal-heading">
        <div>
          <span className="kicker">TEAMRISE WORKSPACE</span>
          <h2 id={titleID}>{title}</h2>
        </div>
        <button
          className="icon-button"
          aria-label="Close dialog"
          onClick={requestClose}
        >
          <X size={20} />
        </button>
      </header>
      {toolbar && <div className="modal-toolbar">{toolbar}</div>}
      <div
        className="modal-scroll"
        key={scrollKey}
        tabIndex={0}
        role="region"
        aria-label="Dialog content"
      >
        {children}
      </div>
      {footer && <div className="modal-action-bar">{footer}</div>}
    </dialog>
  );
}
export function Avatar({
  candidate: c,
  large = false,
}: {
  candidate: Candidate;
  large?: boolean;
}) {
  const [brokenAvatar, setBrokenAvatar] = useState("");
  const avatar = c.avatar_url;
  if (c.mode === "live") {
    return (
      <span
        className={`person-avatar ${large ? "large" : ""}`}
        aria-label={`${c.name} avatar`}
        role="img"
      >
        {avatar &&
        avatar !== brokenAvatar &&
        avatar.startsWith("https://cdn.discordapp.com/") ? (
          <img
            src={avatar}
            alt=""
            referrerPolicy="no-referrer"
            onError={() => setBrokenAvatar(avatar)}
          />
        ) : (
          <span className="avatar-initials">
            {c.initials || c.name.slice(0, 1)}
          </span>
        )}
      </span>
    );
  }
  const n = Number(c.id.replace(/\D/g, "")) || 1;
  const skin = ["#dba982", "#d8b09b", "#c58b68", "#edc7ac", "#b78364"][n % 5];
  const shirt = ["#a78bfa", "#60a5fa", "#34d399", "#f4b96b", "#e88cb8"][n % 5];
  return (
    <span
      className={`person-avatar ${large ? "large" : ""}`}
      style={{ background: c.color }}
    >
      <svg viewBox="0 0 80 80" role="img" aria-label={`${c.name} avatar`}>
        <path d="M9 82C9 51 71 51 71 82" fill={shirt} />
        <path d="M29 48h22v17H29z" fill={skin} />
        <ellipse cx="40" cy="32" rx="20" ry="24" fill={skin} />
        <path
          d={
            n % 2
              ? "M20 30C10 0 64-8 61 32L53 20 42 19 32 26z"
              : "M20 36C6 4 37-5 54 8 68 17 61 32 60 34L51 17 34 22 27 21z"
          }
          fill="#38313f"
        />
        <circle cx="33" cy="33" r="1.5" fill="#39313b" />
        <circle cx="48" cy="33" r="1.5" fill="#39313b" />
        <path
          d="M35 45q6 5 12-1"
          fill="none"
          stroke="#975b4a"
          strokeWidth="1.8"
          strokeLinecap="round"
        />
      </svg>
    </span>
  );
}
export function SolMark({ small = false }: { small?: boolean }) {
  return (
    <span className={`sol-mark ${small ? "small" : ""}`}>
      <svg
        width={small ? 17 : 25}
        height={small ? 17 : 25}
        viewBox="0 0 24 24"
        fill="none"
        aria-hidden="true"
      >
        <path
          d="M4 19v-5m7 5V9m7 10V4"
          stroke="currentColor"
          strokeWidth="3.5"
          strokeLinecap="round"
        />
        <path
          d="m3 8 6-4 4 1 6-4"
          stroke="currentColor"
          strokeWidth="1.5"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </svg>
    </span>
  );
}
export function interviewFailed(c: Candidate) {
  return c.stage === "Rejected" || c.live?.status === "failed";
}

export function StatusBadge({
  tone = "quiet",
  children,
}: {
  tone?: "quiet" | "ready" | "active" | "failed";
  children: React.ReactNode;
}) {
  const Icon =
    tone === "ready" ? Check : tone === "failed" ? CircleAlert : Circle;
  return (
    <span className={`status-badge ${tone}`}>
      <Icon
        size={tone === "quiet" || tone === "active" ? 8 : 13}
        aria-hidden="true"
      />
      {children}
    </span>
  );
}

export function stageTone(stage: string) {
  return stage === "Review"
    ? "amber"
    : stage === "Hired"
      ? "green"
      : stage === "Rejected"
        ? "rose"
        : stage === "Final"
          ? "purple"
          : stage === "Interviewing"
            ? "blue"
            : "neutral";
}
