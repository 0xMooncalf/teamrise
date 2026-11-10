import { spawn } from 'node:child_process';
import { createInterface } from 'node:readline';
import { Readable } from 'node:stream';
import { fileURLToPath } from 'node:url';
import { dirname, join } from 'node:path';
import { createAudioPlayer, createAudioResource, joinVoiceChannel, entersState, VoiceConnectionStatus, AudioPlayerStatus, StreamType, EndBehaviorType, NoSubscriberBehavior } from '@discordjs/voice';
import OpusScript from 'opusscript';
import { retryIPDiscovery } from './discovery.mjs';

const root = dirname(fileURLToPath(import.meta.url));
const emit = (value) => process.stdout.write(JSON.stringify(value) + '\n');
const python = process.env.TEAMRISE_VOICE_PYTHON || join(root, '.venv', process.platform === 'win32' ? 'Scripts/python.exe' : 'bin/python');
const audio = spawn(python, [join(root, 'audio_service.py')], { cwd: root, windowsHide: true, stdio: ['pipe', 'pipe', 'ignore'] });
let pending = new Map(), counter = 0, active = null;
let audioQueue = Promise.resolve();
const safeCode = (value) => typeof value === 'number' ? String(value) : typeof value === 'string' && /^[A-Z_0-9]{1,40}$/.test(value) ? value : 'none';
let lastJoin = null;
const askAudio = (type, data, timeout = 90000) => {
  const job = () => new Promise((resolve, reject) => {
    const id = ++counter;
    const timer = setTimeout(() => { pending.delete(id); reject(Error('Local speech timed out')); }, timeout);
    pending.set(id, { resolve, reject, timer });
    audio.stdin.write(JSON.stringify({ id, type, ...data }) + '\n');
  });
  const result = audioQueue.then(job); audioQueue = result.catch(() => {}); return result;
};
createInterface({ input: audio.stdout }).on('line', (line) => {
  try {
    const result = JSON.parse(line);
    if (result.event === 'ready') { emit({ event: 'ready', language: 'en', asr: result.asr, tts: result.tts }); return; }
    if (result.event === 'fatal') { emit({ event: 'fatal', error: result.error }); return; }
    const job = pending.get(result.id); if (!job) return;
    clearTimeout(job.timer); pending.delete(result.id);
    if (result.error) job.reject(Error(result.error)); else job.resolve(result);
  } catch { emit({ event: 'fatal', error: 'Invalid local speech response' }); }
});
audio.on('error', () => emit({ event: 'fatal', error: 'Local speech runtime could not start. Run Install-Voice.ps1.' }));
audio.on('exit', () => { for (const p of pending.values()) { clearTimeout(p.timer); p.reject(Error('Local speech stopped')); } pending.clear(); emit({ event: 'fatal', error: 'Local speech process stopped' }); });

function leave() {
  const s = active; active = null; if (!s) return;
  s.version++; s.listening = false;
  s.stopDiscovery?.();
  for (const stream of s.streams.values()) stream.destroy();
  s.player?.stop(true); s.connection?.destroy();
}
function receive(s, userID) {
  if (active !== s || userID !== s.memberID || !s.listening || s.streams.has(userID)) return;
  s.version++; s.player.stop(true);
  const epoch = s.epoch;
  const input = s.connection.receiver.subscribe(userID, { end: { behavior: EndBehaviorType.AfterSilence, duration: 1400 } });
  s.streams.set(userID, input);
  const decoder = new OpusScript(48000, 2, OpusScript.Application.AUDIO);
  let chunks = [], size = 0, nextActivity = 57600, finished = false;
  const finish = async () => {
    if (finished) return; finished = true;
    clearTimeout(timer); s.streams.delete(userID); decoder.delete();
    if (active !== s || !s.listening || epoch !== s.epoch || size < 19200) { chunks = []; return; }
    s.listening = false;
    emit({ event: 'transcribing', session_id: s.id, epoch });
    try {
      const result = await askAudio('transcribe', { pcm: Buffer.concat(chunks).toString('base64') }); chunks = [];
      if (active !== s || epoch !== s.epoch) return;
      emit({ event: result.text ? 'transcript' : 'empty', session_id: s.id, epoch, text: result.text, seconds: result.seconds, duration_ms: result.duration_ms });
    } catch { if (active === s) emit({ event: 'speech.error', session_id: s.id, error: 'Could not transcribe the answer; repeat or use /interview-repeat.' }); }
  };
  const timer = setTimeout(() => { input.destroy(); void finish(); }, 90000);
  input.on('data', (packet) => {
    try {
      const pcm = Buffer.from(decoder.decode(packet));
      size += pcm.length;
      if (size >= nextActivity && active === s && s.listening && epoch === s.epoch) {
        nextActivity = size + 5 * 192000;
        emit({ event: 'voice.activity', session_id: s.id, epoch });
      }
      if (size <= 90 * 192000) chunks.push(pcm); else { input.destroy(); void finish(); }
    }
    catch { /* Corrupt packets are not fabricated as speech. */ }
  });
  input.on('end', () => void finish()); input.on('error', () => void finish()); input.on('close', () => void finish());
}
async function say(s, cmd) {
  const version = ++s.version;
  s.epoch = cmd.epoch ?? s.epoch; s.listening = Boolean(cmd.listen);
  const result = await askAudio('speak', { text: cmd.text, speed: 1.15 });
  if (active !== s || s.version !== version) return { interrupted: true };
  await play(s, Buffer.from(result.pcm, 'base64'), Math.max(30000, (result.seconds + 10) * 1000));
  return { seconds: result.seconds, characters: result.characters, duration_ms: result.duration_ms, interrupted: s.version !== version };
}
async function play(s, pcm, timeout) {
  const resource = createAudioResource(Readable.from([pcm]), { inputType: StreamType.Raw });
  const played = new Promise((resolve, reject) => {
    const end = () => { cleanup(); resolve(); }, error = () => { cleanup(); reject(Error('Voice playback failed')); };
    const timer = setTimeout(error, timeout);
    function cleanup() { clearTimeout(timer); s.player.off(AudioPlayerStatus.Idle, end); s.player.off('error', error); }
    s.player.once(AudioPlayerStatus.Idle, end); s.player.once('error', error);
  });
  s.player.play(resource);
  await played;
}

function recoveryChime() {
  const rate = 48000;
  const pcm = Buffer.alloc(rate * 4 / 2);
  for (const [start, pitch] of [[0, 440], [0.23, 587.33]]) {
    for (let i = 0; i < rate * 0.19; i++) {
      const t = i / rate;
      const gain = Math.min(t / 0.02, (0.19 - t) / 0.04, 1);
      const sample = Math.round(4400 * gain * Math.sin(2 * Math.PI * pitch * t));
      const offset = (Math.round(start * rate) + i) * 4;
      pcm.writeInt16LE(sample, offset);
      pcm.writeInt16LE(sample, offset + 2);
    }
  }
  return pcm;
}

async function handle(cmd) {
  if (cmd.type === 'voice_server' || cmd.type === 'voice_state') {
    if (active?.guildID === cmd.data?.guild_id && active.adapter) {
      if (cmd.type === 'voice_server') { active.sawServer = true; active.adapter.onVoiceServerUpdate(cmd.data); }
      else if (cmd.data.user_id === active.botID) { active.sawState = true; active.adapter.onVoiceStateUpdate(cmd.data); }
    }
    return;
  }
  let result = {};
  if (cmd.type === 'join') {
    if (active) throw Error('Interviewer is busy');
    const s = { id: cmd.session_id, guildID: cmd.guild_id, botID: cmd.bot_id, memberID: cmd.member_id,
      epoch: 0, version: 0, listening: false, adapter: null, streams: new Map(), sawState: false, sawServer: false };
    lastJoin = s;
    active = s;
    s.player = createAudioPlayer({ behaviors: { noSubscriber: NoSubscriberBehavior.Pause } });
    s.player.on('error', () => emit({ event: 'speech.error', session_id: s.id, error: 'Discord audio playback failed' }));
    s.connection = joinVoiceChannel({ guildId: s.guildID, channelId: cmd.channel_id, selfDeaf: false, selfMute: false,
      adapterCreator(methods) { s.adapter = methods; return {
        sendPayload(payload) { if (active !== s) return false; emit({ event: 'gateway.send', session_id: s.id, payload }); return true; },
        destroy() { s.adapter = null; }
      }; }
    });
    s.connection.subscribe(s.player);
    s.stopDiscovery = retryIPDiscovery(s.connection, { onRetry: (attempt) => { s.discoveryRetries = attempt; } });
    s.connection.on('stateChange', (_, state) => { s.transportState = state.status; if (state.networking) s.networkState = state.networking.state.code; if (typeof state.closeCode === 'number') s.closeCode = state.closeCode; });
    s.connection.receiver.speaking.on('start', (userID) => receive(s, userID));
    s.connection.on('error', (error) => {s.errorCode = safeCode(error.code); emit({ event: 'voice.error', session_id: s.id, error: 'Discord voice transport error' });});
    s.connection.on(VoiceConnectionStatus.Disconnected, () => { s.listening = false; emit({ event: 'voice.lost', session_id: s.id }); });
    await entersState(s.connection, VoiceConnectionStatus.Ready, 25000);
    if (!cmd.quiet) emit({ event: 'voice.ready', session_id: s.id });
  } else if (cmd.type === 'leave') { leave(); }
  else if (cmd.type === 'health') {
    if (audio.exitCode !== null || audio.signalCode !== null) throw Error('Local speech stopped');
    // An active speech job has its own timeout. An idle runtime must reply.
    if (pending.size === 0) await askAudio('ping', {}, 3000);
    result.connected = active?.id === cmd.session_id && active?.connection?.state.status === VoiceConnectionStatus.Ready;
  }
  else if (cmd.type === 'chime') {
    const s = active;
    if (s?.id !== cmd.session_id || s?.connection?.state.status !== VoiceConnectionStatus.Ready) throw Error('Voice is not connected');
    s.listening = false;
    s.version++;
    await play(s, recoveryChime(), 2000);
    if (active !== s || s.connection.state.status !== VoiceConnectionStatus.Ready) throw Error('Chime interrupted');
  }
  else if (cmd.type === 'speak') { if (active?.id !== cmd.session_id) throw Error('Voice session is not active'); result = await say(active, cmd); }
  else if (cmd.type === 'listen') { if (active?.id === cmd.session_id) { active.listening = Boolean(cmd.enabled); active.epoch = cmd.epoch ?? active.epoch; if (!cmd.enabled) { active.version++; active.player.stop(true); } } }
  else if (cmd.type === 'shutdown') { leave(); audio.kill(); process.exit(0); }
  else throw Error('Unsupported voice operation');
  if (cmd.id) emit({ event: 'ack', id: cmd.id, ...result });
}
createInterface({ input: process.stdin }).on('line', (line) => {
  let cmd;
  try { if (line.length > 100000) throw Error(); cmd = JSON.parse(line); }
  catch { emit({ event: 'fatal', error: 'Invalid voice control message' }); return; }
  void handle(cmd).catch((error) => {
    let detail = 'Voice operation failed. Check Discord voice permissions, UDP connectivity and local speech setup.';
    if (cmd.type === 'join') {
      const s = lastJoin;
      const net = s?.connection?.state.networking?.state;
      detail = `Discord voice handshake failed: ${s?.connection?.state.status || 'initializing'}; voice-state=${Boolean(s?.sawState)}, voice-server=${Boolean(s?.sawServer)}, network=${safeCode(net?.code ?? s?.networkState)}, UDP=${safeCode(net?.udp?.remote?.port)}, retries=${s?.discoveryRetries || 0}, close=${safeCode(s?.closeCode)}, code=${s?.errorCode || safeCode(error.code)}.`;
      leave();
    }
    emit({ event: 'ack', id: cmd.id, error: detail });
  });
}).on('close', () => { leave(); audio.kill(); process.exit(0); });
process.on('SIGTERM', () => { leave(); audio.kill(); process.exit(0); });
