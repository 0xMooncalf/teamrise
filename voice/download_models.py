"""Download ready-made English models; never uses a user's API key."""
import json
from pathlib import Path
from huggingface_hub import HfApi, hf_hub_download, snapshot_download

root = Path(__file__).resolve().parent / "models"
root.mkdir(exist_ok=True)
api = HfApi()
asr_repo = "Systran/faster-whisper-base.en"
tts_repo = "rhasspy/piper-voices"
asr_revision = "3d3d5dee26484f91867d81cb899cfcf72b96be6c"
tts_revision = "e7229f7d33a10e145e109b886d661984201ac3c4"
snapshot_download(asr_repo, revision=asr_revision, local_dir=root / "whisper-base.en",
                  allow_patterns=["config.json", "model.bin", "tokenizer.json", "vocabulary.*", "preprocessor_config.json"])
voice_dir = "en/en_US/ljspeech/medium/"
for name in ["en_US-ljspeech-medium.onnx", "en_US-ljspeech-medium.onnx.json", "MODEL_CARD"]:
    downloaded = Path(hf_hub_download(tts_repo, filename=voice_dir + name, revision=tts_revision))
    (root / name).write_bytes(downloaded.read_bytes())
(root / "manifest.json").write_text(json.dumps({"asr": {"repository": asr_repo, "revision": asr_revision},
    "tts": {"repository": tts_repo, "revision": tts_revision, "voice": "en_US-ljspeech-medium"}}, indent=2), encoding="utf-8")
print("English speech models ready.", flush=True)
