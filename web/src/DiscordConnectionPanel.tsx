import GlassSelect from "./GlassSelect";
import { StatusBadge } from "./shared";
import { useEffect, useState, type Dispatch, type SetStateAction } from "react";
import {
  Check,
  ExternalLink,
  KeyRound,
  Link2,
  LockKeyhole,
  Plug,
  ShieldCheck,
  Unplug,
  X,
} from "lucide-react";
import type {
  DiscordConfig,
  DiscordConnection,
  State,
  VoiceConnection,
} from "./types";

type Details = {
  guild_id: string;
  categories: { id: string; name: string }[];
  roles: { id: string; name: string }[];
  checks: { name: string; granted: boolean }[];
};

async function post<T>(path: string, body: object): Promise<T> {
  const response = await fetch(path, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
  const result = await response.json();
  if (!response.ok) throw Error(result.error || "Could not reach Discord.");
  return result;
}

export default function DiscordConnectionPanel({
  config,
  setConfig,
  connection,
  voice,
}: {
  config: DiscordConfig;
  setConfig: Dispatch<SetStateAction<DiscordConfig>>;
  connection?: DiscordConnection;
  voice?: VoiceConnection;
}) {
  const [token, setToken] = useState("");
  const [busy, setBusy] = useState(false);
  const [loading, setLoading] = useState(false);
  const [details, setDetails] = useState<Details | null>(null);
  const [message, setMessage] = useState("");
  const [failed, setFailed] = useState(false);
  const [localStatus, setLocalStatus] = useState(connection);
  const [localVoice, setLocalVoice] = useState(voice);
  const speech =
    voice && voice.revision >= (localVoice?.revision ?? 0) ? voice : localVoice;
  const status =
    connection && connection.revision >= (localStatus?.revision ?? 0)
      ? connection
      : localStatus;
  useEffect(() => {
    if (
      !failed &&
      ["error", "voice_error", "connection_lost"].includes(speech?.state || "")
    )
      setMessage("");
  }, [speech?.state, failed]);

  useEffect(() => {
    if (!status?.connected || !config.guild_id) {
      setDetails(null);
      return;
    }
    const controller = new AbortController();
    setLoading(true);
    setDetails(null);
    setMessage("");
    fetch("/api/discord/details", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        guild_id: config.guild_id,
        category_id: config.category_id,
      }),
      signal: controller.signal,
    })
      .then(async (r) => {
        const d = await r.json();
        if (!r.ok) throw Error(d.error);
        return d as Details;
      })
      .then((d) => {
        if (controller.signal.aborted) return;
        setDetails(d);
        const categories = d.categories.filter(
          (o) => o.name.toLowerCase() === "interviews",
        );
        const roles = d.roles.filter(
          (o) => o.name.toLowerCase() === "reviewer",
        );
        setConfig((previous) => ({
          ...previous,
          category_id:
            previous.category_id ||
            (categories.length === 1 ? categories[0].id : ""),
          reviewer_role_id:
            previous.reviewer_role_id ||
            (roles.length === 1 ? roles[0].id : ""),
        }));
      })
      .catch((e) => {
        if (!controller.signal.aborted) {
          setFailed(true);
          setMessage(
            e instanceof Error ? e.message : "Could not load the server.",
          );
        }
      })
      .finally(() => {
        if (!controller.signal.aborted) setLoading(false);
      });
    return () => controller.abort();
  }, [status?.connected, config.guild_id, config.category_id, setConfig]);

  async function connect(reconnect = false) {
    setBusy(true);
    setFailed(false);
    setMessage("");
    const enteredToken = token;
    setToken("");
    try {
      const s = await post<DiscordConnection>(
        reconnect ? "/api/discord/reconnect" : "/api/discord/connect",
        reconnect ? {} : { token: enteredToken },
      );
      setLocalStatus(s);
      setConfig((previous) => {
        const guild = s.guilds.some((g) => g.id === previous.guild_id)
          ? previous.guild_id
          : s.guilds.length === 1
            ? s.guilds[0].id
            : "";
        return {
          ...previous,
          enabled: false,
          bot_user_id: s.bot_id || "",
          display_name: s.bot_name || previous.display_name,
          guild_id: guild,
          category_id: guild === previous.guild_id ? previous.category_id : "",
          reviewer_role_id:
            guild === previous.guild_id ? previous.reviewer_role_id : "",
        };
      });
      setMessage(
        s.connected
          ? "Bot connected. Choose your server, category and reviewer role below."
          : s.message,
      );
    } catch (e) {
      setFailed(true);
      setMessage(e instanceof Error ? e.message : "Could not connect the bot.");
    } finally {
      setBusy(false);
    }
  }
  async function disconnect() {
    setBusy(true);
    setFailed(false);
    setMessage("");
    try {
      const s = await post<DiscordConnection>("/api/discord/disconnect", {});
      setLocalStatus(s);
      setDetails(null);
      setMessage("Bot disconnected. Token removed from memory.");
    } catch (e) {
      setFailed(true);
      setMessage(e instanceof Error ? e.message : "Could not disconnect.");
    } finally {
      setBusy(false);
    }
  }
  async function save() {
    setBusy(true);
    setFailed(false);
    setMessage("");
    try {
      await post("/api/discord/configure", {
        ...config,
        enabled: true,
        auto_create_rooms: true,
        bot_user_id: status?.bot_id || config.bot_user_id,
      });
      setConfig((previous) => ({
        ...previous,
        enabled: true,
        auto_create_rooms: true,
      }));
      setMessage(
        "Discord settings verified and saved. Enable local voice below to register /interview.",
      );
    } catch (e) {
      setFailed(true);
      setMessage(e instanceof Error ? e.message : "Could not verify settings.");
    } finally {
      setBusy(false);
    }
  }
  async function enableVoice() {
    setBusy(true);
    setFailed(false);
    setMessage("Loading local speech and registering Discord commands…");
    try {
      const result = await post<State>("/api/voice/enable", {});
      setLocalVoice(result.voice_connection);
      setMessage(result.voice_connection?.message || "Voice ready.");
    } catch (e) {
      setFailed(true);
      setMessage(e instanceof Error ? e.message : "Voice could not start.");
    } finally {
      setBusy(false);
    }
  }
  const canSave =
    status?.connected &&
    config.guild_id &&
    config.category_id &&
    config.reviewer_role_id &&
    details &&
    details.checks.every((check) => check.granted);
  return (
    <div className="discord-connection-panel">
      <div className="connection-intro">
        <span className="integration-icon">
          <Plug size={23} />
        </span>
        <div>
          <h3>Connect your interviewer.</h3>
          <p>One visible Discord bot. Your AI team works behind the scenes.</p>
        </div>
        <StatusBadge
          tone={
            status?.connected
              ? "ready"
              : status?.gateway === "connecting"
                ? "active"
                : "quiet"
          }
        >
          {status?.connected
            ? "Gateway connected"
            : status?.gateway === "connecting"
              ? "Connecting…"
              : "Not connected"}
        </StatusBadge>
      </div>
      <div className="discord-token-box">
        <label className="form-field">
          <span>
            <KeyRound size={14} /> Discord bot token · session only
          </span>
          <input
            type="password"
            name="discord-bot-token"
            value={token}
            onChange={(e) => setToken(e.target.value)}
            autoComplete="new-password"
            spellCheck={false}
            disabled={busy}
            placeholder={
              status?.has_token
                ? "Token is held in memory · paste only to replace"
                : "Paste the bot token here, never in chat"
            }
          />
          <small>
            <a
              href="https://discord.com/developers/applications"
              target="_blank"
              rel="noreferrer"
            >
              Developer Portal <ExternalLink size={12} />
            </a>{" "}
            → your application → Bot → Reset Token / Copy. Do not use the
            Application ID, Public Key or OAuth client secret.
          </small>
        </label>
        <div className="discord-connect-actions">
          <button
            className="button primary"
            disabled={busy || !token.trim()}
            onClick={() => void connect()}
          >
            <Link2 size={15} />
            {busy
              ? "Working…"
              : status?.has_token
                ? "Replace & connect"
                : "Connect bot"}
          </button>
          {status?.has_token && !status.connected && (
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => void connect(true)}
            >
              Reconnect bot
            </button>
          )}
          {status?.has_token && (
            <button
              className="button secondary"
              disabled={busy}
              onClick={() => void disconnect()}
            >
              <Unplug size={14} />
              Disconnect & forget token
            </button>
          )}
        </div>
        <p className="fine-print">
          <LockKeyhole size={13} /> Sent only to this local Teamrise server and
          Discord. Never saved in settings, browser storage, exports or logs.
          Enter it again after restarting Teamrise.
        </p>
      </div>
      {status?.has_token && (
        <div className="discord-session-summary">
          <strong>{status.bot_name || "Discord bot"}</strong>
          <span>{status.message}</span>
          <small>
            Bot ID: {status.bot_id} · Voice{" "}
            {speech?.state.replaceAll("_", " ") || "disabled"}
          </small>
        </div>
      )}
      {status?.connected && (
        <>
          <div className="connection-fields discord-server-fields">
            <label className="form-field connection-full">
              <span>Discord server</span>
              <GlassSelect
                label="Discord server"
                disabled={busy}
                value={config.guild_id}
                placeholder="Choose a server"
                options={status.guilds.map((g) => ({
                  value: g.id,
                  label: g.name,
                }))}
                onChange={(value) => {
                  setFailed(false);
                  setConfig({
                    ...config,
                    enabled: false,
                    guild_id: value,
                    category_id: "",
                    reviewer_role_id: "",
                  });
                }}
              />
              <small>
                {status.guilds.length
                  ? "Only servers where this bot is installed are listed."
                  : "No servers found. Add this bot to your test server, then reconnect."}
              </small>
            </label>
            <label className="form-field">
              <span>Interview category</span>
              <GlassSelect
                label="Interview category"
                disabled={busy || loading || !details}
                value={config.category_id}
                placeholder={loading ? "Loading…" : "Choose Interviews"}
                options={
                  details?.categories.map((o) => ({
                    value: o.id,
                    label: o.name,
                  })) ?? []
                }
                onChange={(value) =>
                  setConfig({ ...config, enabled: false, category_id: value })
                }
              />
            </label>
            <label className="form-field">
              <span>Reviewer role</span>
              <GlassSelect
                label="Reviewer role"
                disabled={busy || loading || !details}
                value={config.reviewer_role_id}
                placeholder="Choose Reviewer"
                options={
                  details?.roles.map((o) => ({ value: o.id, label: o.name })) ??
                  []
                }
                onChange={(value) =>
                  setConfig({
                    ...config,
                    enabled: false,
                    reviewer_role_id: value,
                  })
                }
              />
            </label>
          </div>
          {details && (
            <div
              className="discord-permission-checks"
              aria-label="Bot permission checks"
            >
              {details.checks.map((check) => (
                <span
                  key={check.name}
                  className={check.granted ? "granted" : "missing"}
                >
                  {check.granted ? <Check size={13} /> : <X size={13} />}
                  {check.name}
                </span>
              ))}
            </div>
          )}
          <div className="discord-connect-actions">
            <button
              className="button primary"
              disabled={busy || loading || !canSave}
              onClick={() => void save()}
            >
              <ShieldCheck size={15} />
              {busy ? "Verifying…" : "Verify & save Discord settings"}
            </button>
          </div>
          {details?.checks.some((c) => !c.granted) && (
            <p className="soft-note">
              Grant the missing permissions to the bot on this server/category,
              then reselect the category to check again. Administrator is not
              required.
            </p>
          )}
        </>
      )}
      <div className="discord-token-box voice-setup">
        <div className="connection-intro">
          <div>
            <h3>Speak with your interviewer.</h3>
            <p>Local English speech · faster-whisper + Piper · Discord DAVE</p>
          </div>
          <StatusBadge
            tone={
              speech?.state.includes("error")
                ? "failed"
                : speech?.ready && status?.connected
                  ? "ready"
                  : "quiet"
            }
          >
            {speech?.state === "recovering_voice"
              ? "Reconnecting"
              : speech?.state.replaceAll("_", " ") || "Disabled"}
          </StatusBadge>
        </div>
        <button
          className="button primary"
          disabled={
            busy ||
            speech?.state === "recovering_voice" ||
            !status?.connected ||
            !config.enabled
          }
          onClick={() => void enableVoice()}
        >
          {busy
            ? "Preparing…"
            : speech?.enabled
              ? "Reconnect voice & refresh skills"
              : "Enable local voice"}
        </button>
        <p className="soft-note">
          {speech?.message ||
            "Connect the bot and save Discord settings first."}
        </p>
        {speech?.channel_id && (
          <a
            className="button secondary"
            href={`https://discord.com/channels/${config.guild_id}/${speech.channel_id}`}
            target="_blank"
            rel="noreferrer"
          >
            Open active voice room <ExternalLink size={13} />
          </a>
        )}
        <ol className="question-plan">
          <li>
            In Discord, run <code>/interview</code> and choose a saved skill.
          </li>
          <li>
            Accept the interview notice, then join your private voice room.
          </li>
          <li>
            Wait for each question and answer in English. A 1.4-second pause
            submits the answer.
          </li>
          <li>
            Open the room chat to answer the written exercise with{" "}
            <code>/interview-answer</code> or request a reviewer with{" "}
            <code>/interview-help</code>.
          </li>
        </ol>
        <p className="fine-print">
          When waiting for a spoken answer, Echo reconnects after five minutes
          of silence and repeats the current question. Written tasks keep
          waiting in the room chat.
        </p>
        <p className="fine-print">
          One bot, one live voice room at a time. Audio is processed in memory;
          the transcript and usage are saved locally. Codex subscription handles
          the text AI; speech needs no API key. The room is removed after the
          candidate is absent for 5 minutes. Returning cancels the timer;
          finishing the AI report does not remove the room.
        </p>
      </div>
      {message && (
        <p
          role="status"
          className={`connection-result ${failed ? "failed" : ""}`}
        >
          {!failed && <Check size={15} />}
          {message}
        </p>
      )}
      <p className="soft-note">
        Other members cannot see interview rooms. The candidate, interviewer and
        Reviewer role have access; server owners and Administrators retain
        access. Keep Teamrise running throughout the interview.
      </p>
    </div>
  );
}
