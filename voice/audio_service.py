"""Local speech subprocess. JSONL on stdio; raw audio is never saved to disk."""
import base64
import io
import json
import os
import sys
import time
import wave
from pathlib import Path

os.environ.setdefault("HF_HUB_OFFLINE", "1")
os.environ.setdefault("HF_HUB_DISABLE_TELEMETRY", "1")
from faster_whisper import WhisperModel
from piper import PiperVoice, SynthesisConfig
import av

ROOT = Path(__file__).resolve().parent

def emit(value):
    print(json.dumps(value, ensure_ascii=True), flush=True)

def speech_bytes(voice, text, speed=1.15):
    output = io.BytesIO()
    with wave.open(output, "wb") as wav:
        voice.synthesize_wav(text, wav, syn_config=SynthesisConfig(length_scale=max(1.0, min(1.5, speed)), volume=0.85))
    output.seek(0)
    chunks = []
    with av.open(output) as source:
        resampler = av.AudioResampler(format="s16", layout="stereo", rate=48000)
        for frame in source.decode(audio=0):
            chunks.extend(resampler.resample(frame))
        chunks.extend(resampler.resample(None))
    return b"".join(frame.to_ndarray().tobytes() for frame in chunks)

def main():
    try:
        voice = PiperVoice.load(str(ROOT / "models/en_US-ljspeech-medium.onnx"))
        asr = WhisperModel(str(ROOT / "models/whisper-base.en"), device="cpu", compute_type="int8", cpu_threads=4, num_workers=1, local_files_only=True)
    except Exception:
        emit({"event": "fatal", "error": "Local speech models could not load. Run Install-Voice.ps1."})
        return
    emit({"event": "ready", "asr": "faster-whisper-base.en", "tts": "piper-ljspeech-medium", "language": "en"})
    for line in sys.stdin:
        request_id = None
        try:
            if len(line) > 25_000_000:
                raise ValueError("Audio request too large")
            request = json.loads(line)
            request_id = request.get("id")
            started = time.monotonic()
            if request.get("type") == "ping":
                emit({"id": request_id})
            elif request.get("type") == "speak":
                text = request.get("text", "")
                if not isinstance(text, str) or not text.strip() or len(text) > 1600:
                    raise ValueError("Speech text must contain 1–1600 characters")
                pcm = speech_bytes(voice, text, request.get("speed", 1.15))
                emit({"id": request_id, "pcm": base64.b64encode(pcm).decode(), "seconds": len(pcm) / 192000,
                      "characters": len(text), "duration_ms": int((time.monotonic() - started) * 1000)})
            elif request.get("type") == "transcribe":
                pcm = base64.b64decode(request.get("pcm", ""), validate=True)
                if len(pcm) < 19200 or len(pcm) > 90 * 192000 or len(pcm) % 4:
                    raise ValueError("Invalid audio length")
                audio = io.BytesIO()
                with wave.open(audio, "wb") as wav:
                    wav.setnchannels(2); wav.setsampwidth(2); wav.setframerate(48000); wav.writeframes(pcm)
                audio.seek(0)
                segments, _ = asr.transcribe(audio, language="en", beam_size=3, condition_on_previous_text=False,
                    vad_filter=True, vad_parameters={"min_silence_duration_ms": 700})
                accepted = [s.text.strip() for s in segments if s.no_speech_prob < 0.6 and s.avg_logprob > -1.0]
                text = " ".join(accepted).strip()
                emit({"id": request_id, "text": text[:16000], "seconds": len(pcm) / 192000,
                      "duration_ms": int((time.monotonic() - started) * 1000)})
            else:
                raise ValueError("Unsupported speech operation")
        except Exception:
            # Do not echo audio, transcript, system paths or underlying library errors.
            emit({"id": request_id, "error": "Local speech processing failed; repeat the message or check voice setup."})

if __name__ == "__main__":
    main()
