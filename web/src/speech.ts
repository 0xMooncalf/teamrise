import { useEffect, useRef, useState } from "react";
import type { Message } from "./types";

export type VoicePrefs = {
  rate: number;
  volume: number;
  interviewer: string;
  candidate: string;
};
export function readLocal<T>(key: string, fallback: T): T {
  try {
    const data = localStorage.getItem(key);
    return data ? JSON.parse(data) : fallback;
  } catch {
    return fallback;
  }
}
export function useSpeech() {
  const supported =
    "speechSynthesis" in window && "SpeechSynthesisUtterance" in window;
  const [voices, setVoices] = useState<SpeechSynthesisVoice[]>([]);
  const [prefs, setPrefs] = useState<VoicePrefs>(() => {
    const saved = readLocal<VoicePrefs>("ladder-voice-v2", {
      rate: 0.78,
      volume: 0.85,
      interviewer: "",
      candidate: "",
    });
    return {
      rate: Math.min(1.2, Math.max(0.6, Number(saved.rate) || 0.78)),
      volume: Number.isFinite(Number(saved.volume))
        ? Math.min(1, Math.max(0, Number(saved.volume)))
        : 0.85,
      interviewer: saved.interviewer || "",
      candidate: saved.candidate || "",
    };
  });
  const [playing, setPlaying] = useState<string>();
  const [paused, setPaused] = useState(false);
  const [error, setError] = useState("");
  const token = useRef(0);
  const utterances = useRef<SpeechSynthesisUtterance[]>([]);
  useEffect(() => {
    try {
      localStorage.setItem("ladder-voice-v2", JSON.stringify(prefs));
    } catch {}
  }, [prefs]);
  useEffect(() => {
    if (!supported) return;
    const load = () => setVoices(window.speechSynthesis.getVoices());
    load();
    window.speechSynthesis.addEventListener("voiceschanged", load);
    return () => {
      token.current++;
      window.speechSynthesis.cancel();
      window.speechSynthesis.removeEventListener("voiceschanged", load);
    };
  }, []);
  function stop() {
    token.current++;
    if (supported) window.speechSynthesis.cancel();
    utterances.current = [];
    setPlaying(undefined);
    setPaused(false);
  }
  function play(messages: Message[], key: string, candidateName: string) {
    stop();
    setError("");
    if (!supported) {
      setError("Speech playback is not available in this browser.");
      return;
    }
    if (!messages.length) return;
    const run = token.current;
    utterances.current = messages.map((message) => {
      const item = new SpeechSynthesisUtterance(message.text);
      const language = /[а-яё]/i.test(message.text) ? "ru" : "en";
      const isCandidate =
        message.speaker === "Candidate" || message.speaker === candidateName;
      const matching = voices.filter((v) =>
        v.lang.toLowerCase().startsWith(language),
      );
      const preferred = isCandidate ? prefs.candidate : prefs.interviewer;
      item.voice =
        voices.find(
          (v) =>
            v.voiceURI === preferred &&
            v.lang.toLowerCase().startsWith(language),
        ) ??
        matching[isCandidate && matching.length > 1 ? 1 : 0] ??
        null;
      item.lang =
        item.voice?.lang ?? `${language}-${language === "ru" ? "RU" : "US"}`;
      item.rate = prefs.rate;
      item.volume = prefs.volume;
      item.pitch = isCandidate ? 1.02 : 0.98;
      return item;
    });
    setPlaying(key);
    // Queue one utterance at a time, allowing a stop to cancel the whole dialogue.
    function next(index: number) {
      if (run !== token.current) return;
      if (index >= utterances.current.length) {
        setPlaying(undefined);
        setPaused(false);
        return;
      }
      const item = utterances.current[index];
      item.onend = () => next(index + 1);
      item.onerror = (event) => {
        if (
          run !== token.current ||
          event.error === "canceled" ||
          event.error === "interrupted"
        )
          return;
        setPlaying(undefined);
        setPaused(false);
        setError(
          "Voice playback failed. Choose another installed voice or try a single message.",
        );
      };
      window.speechSynthesis.speak(item);
    }
    next(0);
  }
  function togglePause() {
    if (!supported || !playing) return;
    if (paused) window.speechSynthesis.resume();
    else window.speechSynthesis.pause();
    setPaused(!paused);
  }
  return {
    supported,
    voices,
    prefs,
    setPrefs,
    playing,
    paused,
    error,
    play,
    stop,
    togglePause,
  };
}
export type Speech = ReturnType<typeof useSpeech>;
