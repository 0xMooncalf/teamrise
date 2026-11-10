import {
  ArrowRight,
  AudioLines,
  Bot,
  MessageSquare,
  Pause,
  Play,
  Sparkles,
  Info,
  Square,
  Volume2,
} from "lucide-react";
import type { Candidate } from "./types";
import type { Speech } from "./speech";

export function Transcript({
  candidate: c,
  speech,
  maxMessages = Infinity,
}: {
  candidate: Candidate;
  speech: Speech;
  maxMessages?: number;
}) {
  const dialogue = c.transcript.filter(
    (message) => message.speaker !== "System" && message.channel !== "chat",
  );
  return (
    <div className="transcript">
      <div className="transcript-toolbar">
        <span>{c.transcript.length} transcript entries</span>
        <button
          className="text-button"
          disabled={!dialogue.length || !speech.supported}
          onClick={() =>
            speech.playing?.startsWith(c.id)
              ? speech.stop()
              : speech.play(dialogue, c.id, c.name)
          }
        >
          {speech.playing?.startsWith(c.id) ? (
            <Square size={13} />
          ) : (
            <AudioLines size={14} />
          )}{" "}
          {speech.playing?.startsWith(c.id) ? "Stop" : "Play dialogue"}
        </button>
        {speech.playing?.startsWith(c.id) && (
          <button
            className="text-button"
            onClick={speech.togglePause}
            aria-label={
              speech.paused ? "Resume interview audio" : "Pause interview audio"
            }
          >
            {speech.paused ? <Play size={13} /> : <Pause size={13} />}{" "}
            {speech.paused ? "Resume" : "Pause"}
          </button>
        )}
      </div>
      {c.transcript.length ? (
        c.transcript
          .map((message, i) => ({ message, i }))
          .slice(-maxMessages)
          .map(({ message, i }) =>
            message.speaker === "System" ? (
              <div className="transcript-system" key={i}>
                <Info size={14} aria-hidden="true" />
                <div>
                  <strong>System</strong>
                  <p>{message.text}</p>
                </div>
              </div>
            ) : (
              <div
                key={i}
                className={`transcript-message ${message.speaker === "Interviewer" ? "interviewer" : "candidate"}`}
              >
                <div>
                  <strong>
                    {message.speaker === "Interviewer"
                      ? c.mode === "live"
                        ? "Echo · AI Interviewer"
                        : "Sol · Interviewer"
                      : c.name}
                  </strong>
                  {message.channel === "chat" ? (
                    <span className="transcript-channel">Room chat</span>
                  ) : (
                    <button
                      className="message-play"
                      aria-label={`Play message ${i + 1} from ${message.speaker}`}
                      title="Play this message only"
                      disabled={!speech.supported}
                      onClick={() =>
                        speech.playing === `${c.id}-${i}`
                          ? speech.stop()
                          : speech.play([message], `${c.id}-${i}`, c.name)
                      }
                    >
                      {speech.playing === `${c.id}-${i}` ? (
                        <Volume2 size={13} />
                      ) : (
                        <Play size={12} />
                      )}
                    </button>
                  )}
                </div>
                {message.channel === "chat" ? (
                  message.text
                    .split(/```[^\n`]*\n([\s\S]*?)```/g)
                    .map((part, n) =>
                      n % 2 ? (
                        <pre key={n}>
                          <code>{part.trimEnd()}</code>
                        </pre>
                      ) : (
                        part.trim() && <p key={n}>{part}</p>
                      ),
                    )
                ) : (
                  <p>{message.text}</p>
                )}
                {message.image && (
                  <a
                    className="transcript-screenshot"
                    href={"/api/screenshots/" + message.image}
                    target="_blank"
                    rel="noreferrer"
                  >
                    <img
                      src={"/api/screenshots/" + message.image}
                      alt="Practical exercise screenshot"
                    />
                  </a>
                )}
                {message.response_seconds > 0 && (
                  <small>
                    {c.mode === "live"
                      ? "Observed reply time"
                      : "Modeled reply delay"}{" "}
                    · {message.response_seconds}s
                  </small>
                )}
              </div>
            ),
          )
      ) : (
        <div className="card-empty">
          <MessageSquare size={24} />
          <strong>The conversation is coming.</strong>
          <p>
            Questions from this candidate’s skill profile appear here as
            Interviewer works.
          </p>
        </div>
      )}
    </div>
  );
}

export function AIReport({
  candidate: c,
  compact = false,
}: {
  candidate: Candidate;
  compact?: boolean;
}) {
  if (!c.assessment)
    return (
      <div className="card-empty">
        <Sparkles size={24} />
        <strong>Evidence before conclusions.</strong>
        <p>
          Analyst will summarize the answers, observed response times, and a
          recommended next step.
        </p>
      </div>
    );
  return (
    <div className="ai-report">
      <div className="report-block">
        <span className="section-label">
          <Bot size={13} />
          ANALYST’S SUMMARY
        </span>
        <p>{c.assessment.summary}</p>
      </div>
      <div className="report-block">
        <span className="section-label">
          {c.mode === "live"
            ? "OBSERVED IN THIS INTERVIEW"
            : "OBSERVED IN THIS SCENARIO"}
        </span>
        <p>{c.assessment.characterization}</p>
      </div>
      <div className="recommendation">
        <span>
          <ArrowRight size={14} />
          RECOMMENDED NEXT STEP
        </span>
        <p>{c.assessment.recommendation}</p>
      </div>
      {!compact && (
        <>
          <div className="report-columns">
            <div>
              <h4>Strengths</h4>
              <ul>
                {c.assessment.strengths.map((s) => (
                  <li key={s}>{s}</li>
                ))}
              </ul>
            </div>
            <div>
              <h4>Ask next</h4>
              <ul>
                {c.assessment.followups.map((s) => (
                  <li key={s}>{s}</li>
                ))}
              </ul>
            </div>
          </div>
          <div className="scorecard">
            {c.scorecard.map((score) => (
              <div className="score-row" key={score.label}>
                <div>
                  <strong>{score.label}</strong>
                  <span>
                    {score.not_scored
                      ? "Not enough evidence"
                      : `${score.value}/100`}
                  </span>
                </div>
                <div className="score-track">
                  <i
                    style={{ width: `${score.not_scored ? 0 : score.value}%` }}
                  />
                </div>
                <blockquote>{score.evidence}</blockquote>
              </div>
            ))}
          </div>
        </>
      )}
      <div className="fine-print">
        {c.mode === "live"
          ? "AI-generated assessment based on supplied answers. Review the evidence and limitations. Final decisions belong to a human."
          : "Scripted demo observations and scores. Final decisions belong to a human."}
      </div>
    </div>
  );
}
