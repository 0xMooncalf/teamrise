import GlassSelect from "./GlassSelect";
import InterviewPreferences from "./InterviewPreferences";
import { useEffect, useId, useState } from "react";
import {
  Check,
  ExternalLink,
  KeyRound,
  LockKeyhole,
  Plug,
  Save,
} from "lucide-react";
import { Modal } from "./shared";
import AgentAvatar, { agentIdentity } from "./AgentAvatar";
import Tabs from "./Tabs";
import DiscordConnectionPanel from "./DiscordConnectionPanel";
import type { State, Action, Candidate } from "./types";

export function ChannelBadge({ candidate: c }: { candidate?: Candidate }) {
  const room = c?.live?.discord;
  if (room?.channel_id && !c?.room.expired)
    return (
      <a
        className="channel-badge"
        href={`https://discord.com/channels/${room.guild_id}/${room.channel_id}`}
        target="_blank"
        rel="noreferrer"
      >
        <LockKeyhole size={12} />
        Private Discord room
        <ExternalLink size={11} />
      </a>
    );
  return (
    <span className="channel-badge">
      <LockKeyhole size={12} />
      {c?.room.expired && room?.channel_id
        ? "Discord room closed"
        : room
          ? "Discord permissions preview"
          : "Web interview"}
    </span>
  );
}

export default function ConnectionsPanel({
  state,
  action,
  close,
}: {
  state: State;
  action: Action;
  close: () => void;
}) {
  const modelFieldID = useId();
  const [ai, setAI] = useState({ ...state.connections.ai });
  const [discord, setDiscord] = useState({ ...state.connections.discord });
  const [key, setKey] = useState("");
  const [clearKey, setClearKey] = useState(false);
  const [busy, setBusy] = useState(false);
  const [message, setMessage] = useState("");
  const [failed, setFailed] = useState(false);
  const [tab, setTab] = useState("ai");
  const [codex, setCodex] = useState<{
    installed: boolean;
    signed_in: boolean;
    plan?: string;
    models: string[];
    message: string;
  } | null>(null);
  const [checkingCodex, setCheckingCodex] = useState(false);
  const isCodex = ai.provider === "codex";
  async function checkCodex() {
    setCheckingCodex(true);
    try {
      const res = await fetch("/api/connections/codex/status", {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: "{}",
      });
      if (!res.ok) throw Error("Could not check local Codex.");
      setCodex(await res.json());
    } catch {
      setCodex({
        installed: false,
        signed_in: false,
        models: [],
        message:
          "Could not check local Codex. Keep Teamrise running and retry.",
      });
    } finally {
      setCheckingCodex(false);
    }
  }
  useEffect(() => {
    if (state.connections.ai.provider === "codex") void checkCodex();
  }, []);
  function provider(field: "model" | "base_url", value: string) {
    setAI((a) => ({
      ...a,
      [field]: value,
      input_usd_per_million: null,
      cached_input_usd_per_million: null,
      output_usd_per_million: null,
    }));
    setMessage("");
  }
  async function save(test = false) {
    setBusy(true);
    setMessage("");
    setFailed(false);
    try {
      const ok = await action("/api/connections", {
        ai,
        discord,
        bots: state.connections.bots,
        api_key: key,
        clear_key: clearKey,
      });
      if (!ok)
        throw Error(
          "Could not save. Check the API URL, question limit and Discord IDs.",
        );
      setKey("");
      setClearKey(false);
      if (test) {
        const res = await fetch("/api/connections/test", {
          method: "POST",
          headers: { "Content-Type": "application/json" },
          body: "{}",
        });
        const result = await res.json();
        if (!res.ok) throw Error(result.error);
        setMessage(
          isCodex
            ? "Codex responded using your ChatGPT subscription."
            : "The selected AI model responded successfully.",
        );
      } else
        setMessage(
          "Settings saved. Voice status is available on the Discord tab.",
        );
    } catch (e) {
      setFailed(true);
      setMessage(e instanceof Error ? e.message : "Could not reach Teamrise.");
    } finally {
      setBusy(false);
    }
  }
  const rateFields = [
    {
      key: "input_usd_per_million",
      label: "Input tokens",
      hint: "USD per 1M input tokens",
    },
    {
      key: "cached_input_usd_per_million",
      label: "Cached input",
      hint: "Optional · otherwise the input rate is used",
    },
    {
      key: "output_usd_per_million",
      label: "Output tokens",
      hint: "USD per 1M output tokens",
    },
  ] as const;
  return (
    <Modal
      title="Connections & agents"
      close={close}
      className="connections-dialog"
      wide
      scrollKey={tab}
      toolbar={
        <Tabs
          label="Connection settings"
          value={tab}
          change={(t) => {
            setTab(t);
            setMessage("");
          }}
          items={[
            { id: "ai", label: "AI provider" },
            { id: "discord", label: "Discord" },
            { id: "team", label: "Agent team" },
            { id: "pricing", label: "Pricing" },
          ]}
        />
      }
      footer={
        tab !== "discord" && (
          <footer className="connection-footer">
            <span>
              <span className="connection-dot" />
              {isCodex
                ? "Local Codex · subscription"
                : state.connections.has_key
                  ? "Key in memory"
                  : "No key set"}{" "}
              ·
              {state.discord_connection?.connected
                ? state.voice_connection?.ready
                  ? "Discord voice ready"
                  : "Discord connected · enable voice"
                : "Discord offline"}
            </span>
            <div>
              {tab === "ai" && (
                <button
                  className="button secondary"
                  disabled={busy || !ai.model}
                  onClick={() => save(true)}
                >
                  {busy ? "Working…" : "Save & test"}
                </button>
              )}
              <button
                className="button primary"
                disabled={busy}
                onClick={() => save()}
              >
                <Save size={15} />
                {busy ? "Saving…" : "Save settings"}
              </button>
            </div>
          </footer>
        )
      }
    >
      <div className="settings-content tab-panel" key={tab} role="tabpanel">
        {tab === "ai" ? (
          <>
            <div className="connection-intro">
              <span className="integration-icon">
                <Plug size={23} />
              </span>
              <div>
                <h3>A model that fits your team.</h3>
                <p>
                  {isCodex
                    ? "Use the Codex client signed in on this computer. No API key is needed."
                    : "Use your chosen OpenAI-compatible service for interview questions and evidence-based reports."}
                </p>
              </div>
            </div>
            <Tabs
              label="AI connection type"
              value={isCodex ? "codex" : "compatible"}
              change={(value) => {
                setAI({
                  ...ai,
                  provider: value as "codex" | "compatible",
                  model: value === "codex" ? "gpt-6-sol" : "",
                  input_usd_per_million: null,
                  cached_input_usd_per_million: null,
                  output_usd_per_million: null,
                });
                setKey("");
                setMessage("");
                if (value === "codex") void checkCodex();
              }}
              items={[
                { id: "codex", label: "Codex subscription" },
                { id: "compatible", label: "API key" },
              ]}
            />
            {isCodex && (
              <div className="codex-connection-status">
                <div>
                  <strong>
                    {checkingCodex
                      ? "Checking local Codex…"
                      : codex?.signed_in
                        ? "ChatGPT subscription connected"
                        : "Connect your local Codex"}
                  </strong>
                  <p>
                    {codex?.message ||
                      "Open Codex on this computer and sign in with ChatGPT, then check the connection."}
                  </p>
                  {codex?.signed_in && (
                    <small>
                      {codex.plan ? `${codex.plan} plan · ` : ""}Official Codex
                      client on this computer
                    </small>
                  )}
                </div>
                <button
                  className="button secondary"
                  disabled={checkingCodex || busy}
                  onClick={() => void checkCodex()}
                >
                  {checkingCodex ? "Checking…" : "Check connection"}
                </button>
              </div>
            )}
            <div className="connection-fields">
              {!isCodex && (
                <>
                  <label className="form-field">
                    <span>API base URL</span>
                    <input
                      value={ai.base_url}
                      onChange={(e) => provider("base_url", e.target.value)}
                      placeholder="https://your-service.example/v1"
                    />
                    <small>/chat/completions is appended automatically.</small>
                  </label>
                </>
              )}
              <div className="form-field">
                <label htmlFor={modelFieldID}>Model ID</label>
                <input
                  id={modelFieldID}
                  value={ai.model}
                  onChange={(e) => provider("model", e.target.value)}
                  placeholder="Exact model name"
                  autoComplete="off"
                />
                {isCodex && !!codex?.models.length && (
                  <GlassSelect
                    label="Available Codex models"
                    value={ai.model}
                    placeholder="Choose a detected model"
                    options={codex.models.map((model) => ({
                      value: model,
                      label: model,
                    }))}
                    onChange={(model) => provider("model", model)}
                  />
                )}
              </div>
              {!isCodex && (
                <>
                  <label className="form-field connection-full">
                    <span>
                      <KeyRound size={13} />
                      API key · session only
                    </span>
                    <input
                      type="password"
                      value={key}
                      onChange={(e) => setKey(e.target.value)}
                      autoComplete="new-password"
                      placeholder={
                        state.connections.has_key
                          ? "Key is set · leave blank to keep"
                          : "Paste your provider key locally"
                      }
                    />
                    <small>
                      Kept in server memory until Teamrise closes. Changing the
                      API URL clears the previous key. Local services may work
                      without one.
                    </small>
                  </label>
                  {state.connections.has_key && (
                    <label className="connection-check connection-full">
                      <input
                        type="checkbox"
                        checked={clearKey}
                        onChange={(e) => setClearKey(e.target.checked)}
                      />
                      Remove the current key when saving
                    </label>
                  )}
                </>
              )}
              <label className="form-field">
                <span>Question limit</span>
                <input
                  type="number"
                  min={1}
                  max={20}
                  value={ai.max_questions}
                  onChange={(e) =>
                    setAI({ ...ai, max_questions: Number(e.target.value) })
                  }
                />
              </label>
            </div>
            {!isCodex && (
              <details className="advanced-settings">
                <summary>API compatibility options</summary>
                <label className="form-field">
                  <span>Output token parameter</span>
                  <GlassSelect
                    label="Output token parameter"
                    value={ai.token_field}
                    onChange={(value) => setAI({ ...ai, token_field: value })}
                    options={[
                      { value: "max_tokens", label: "max_tokens" },
                      {
                        value: "max_completion_tokens",
                        label: "max_completion_tokens",
                      },
                    ]}
                  />
                </label>
                <label className="connection-check">
                  <input
                    type="checkbox"
                    checked={ai.json_mode}
                    onChange={(e) =>
                      setAI({ ...ai, json_mode: e.target.checked })
                    }
                  />
                  Request JSON response mode (if supported)
                </label>
              </details>
            )}
            <p className="soft-note">
              {isCodex
                ? "Save & test uses one short Codex turn. Interviews use your subscription allowance and send the skill and transcript to OpenAI. One local request runs at a time. This mode is for a local prototype; hosted or commercial services need a supported API integration. Voice is a separate connection."
                : "Save & test sends one short API request. Saving alone does not call the model. The selected service receives your skill prompt and conversation."}
            </p>
          </>
        ) : tab === "discord" ? (
          <>
            <DiscordConnectionPanel
              config={discord}
              setConfig={setDiscord}
              connection={state.discord_connection}
              voice={state.voice_connection}
            />
            <InterviewPreferences
              settings={state.connections.interview}
              action={action}
            />
          </>
        ) : tab === "team" ? (
          <>
            <div className="connection-intro">
              <div>
                <span className="eyebrow">
                  ONE INTERVIEWER · SEVEN SPECIALTIES
                </span>
                <h3>A small team with distinct jobs.</h3>
                <p>
                  These are workflow roles, not seven Discord users. Interviewer
                  and Analyst call your model. Other roles coordinate or
                  validate the work in application code.
                </p>
              </div>
            </div>
            <div className="internal-agent-list">
              {state.agents.map((a) => (
                <article key={a.name}>
                  <span style={{ color: a.color }}>
                    <AgentAvatar agent={a.name} size={39} />
                  </span>
                  <div>
                    <h4>
                      {agentIdentity(a.name).nickname}
                      <span>{a.name}</span>
                    </h4>
                    <p>{a.role}</p>
                  </div>
                  <span
                    className={`agent-kind ${a.name === "Interviewer" ? "public" : ""}`}
                  >
                    {a.name === "Interviewer"
                      ? "Candidate-facing"
                      : a.name === "Analyst"
                        ? "Internal AI"
                        : "Internal workflow"}
                  </span>
                </article>
              ))}
            </div>
          </>
        ) : (
          <>
            {isCodex && (
              <p className="soft-note">
                Codex subscription calls report tokens, not an API invoice.
                These rates apply only to API calls; no dollar estimate is
                assigned to subscription turns.
              </p>
            )}
            <div className="connection-intro">
              <div>
                <span className="eyebrow">PROVIDER RATES · USD</span>
                <h3>Know what each conversation costs.</h3>
                <p>
                  Enter your provider’s rates for{" "}
                  <strong>{ai.model || "the selected model"}</strong>. We record
                  a rate snapshot with each attempt so later edits do not
                  rewrite past estimates.
                </p>
              </div>
            </div>
            <div className="pricing-fields">
              {rateFields.map((field) => (
                <label className="form-field" key={field.key}>
                  <span>{field.label}</span>
                  <div className="rate-input">
                    <span>$</span>
                    <input
                      type="number"
                      min={0}
                      max={1000000}
                      step="any"
                      value={ai[field.key] ?? ""}
                      onChange={(e) =>
                        setAI({
                          ...ai,
                          [field.key]:
                            e.target.value === ""
                              ? null
                              : Number(e.target.value),
                        })
                      }
                      placeholder="Not configured"
                    />
                  </div>
                  <small>{field.hint}</small>
                </label>
              ))}
            </div>
            <p className="soft-note">
              Token counts come from your API response. Monetary amounts are
              estimates from these rates, not provider invoices. Blank means
              unknown; 0 means a configured free rate. Changing the model or API
              URL clears draft rates.
            </p>
            <div className="pricing-formula">
              <span>ESTIMATE PER CALL</span>
              <p>
                Uncached input × input rate + cached input × cached rate +
                output × output rate
              </p>
              <small>
                Rates are per 1,000,000 tokens. Reasoning tokens are already
                included in output.
              </small>
            </div>
          </>
        )}
        {message && (
          <p
            role="status"
            className={`connection-result ${failed ? "failed" : ""}`}
          >
            {!failed && <Check size={16} />} {message}
          </p>
        )}
      </div>
    </Modal>
  );
}
