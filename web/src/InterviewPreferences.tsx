import { useEffect, useState, type FormEvent } from "react";
import { AudioLines, Save, Timer } from "lucide-react";
import type { Action, InterviewSettings } from "./types";

const defaultGreeting =
  "Hello and welcome. I'm Teamrise, your interview assistant.";

export default function InterviewPreferences({
  settings,
  action,
}: {
  settings: InterviewSettings;
  action: Action;
}) {
  const [minutes, setMinutes] = useState(String(settings.minutes));
  const [greeting, setGreeting] = useState(settings.greeting);
  const [saving, setSaving] = useState(false);
  const [status, setStatus] = useState("");
  useEffect(() => {
    setMinutes(String(settings.minutes));
    setGreeting(settings.greeting);
  }, [settings.minutes, settings.greeting]);

  async function save(e: FormEvent<HTMLFormElement>) {
    e.preventDefault();
    setSaving(true);
    setStatus("");
    try {
      const ok = await action("/api/interview-settings", {
        minutes: Number(minutes),
        greeting,
      });
      setStatus(
        ok ? "Saved for new interviews." : "Could not save these settings.",
      );
    } finally {
      setSaving(false);
    }
  }

  return (
    <form className="interview-preferences" onSubmit={save}>
      <div className="connection-intro">
        <span className="integration-icon">
          <AudioLines size={21} />
        </span>
        <div>
          <h3>Interview timing & voice intro</h3>
          <p>
            Set the limit for new live interviews and the opening line Teamrise
            speaks in Discord.
          </p>
        </div>
      </div>
      <label className="form-field interview-minutes">
        <span>
          <Timer size={15} /> Interview time limit (minutes)
        </span>
        <input
          type="number"
          min={1}
          max={480}
          step={1}
          required
          value={minutes}
          onChange={(e) => setMinutes(e.target.value)}
        />
        <small>
          Default 75 minutes. The clock starts with the interview and keeps
          running after a reconnect.
        </small>
      </label>
      <label className="form-field">
        <span>Teamrise voice greeting</span>
        <textarea
          rows={3}
          maxLength={600}
          value={greeting}
          onChange={(e) => setGreeting(e.target.value)}
          placeholder={defaultGreeting}
        />
        <small>
          Teamrise adds the skill, question count and time limit. Leave blank to
          use the default greeting.
        </small>
      </label>
      <div className="interview-preference-actions">
        <button type="submit" className="button primary" disabled={saving}>
          <Save size={15} /> {saving ? "Saving…" : "Save interview settings"}
        </button>
        {status && <span role="status">{status}</span>}
      </div>
    </form>
  );
}
