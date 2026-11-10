"""Opt-in installed-model smoke test: python voice/test_speech.py."""
import json
import subprocess
import sys
from pathlib import Path

root = Path(__file__).resolve().parent
worker = subprocess.Popen([sys.executable, '-u', str(root/'audio_service.py')], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.DEVNULL, text=True, encoding='utf-8')
def read():
    result=json.loads(worker.stdout.readline())
    assert not result.get('error'),result.get('error')
    return result
def ask(value):
    worker.stdin.write(json.dumps(value)+'\n');worker.stdin.flush();return read()
try:
    assert read()['event']=='ready'
    phrase='I ask for requirements, break the task into steps, and check my work before submitting it.'
    spoken=ask({'id':1,'type':'speak','text':phrase})
    heard=ask({'id':2,'type':'transcribe','pcm':spoken['pcm']})
    assert spoken['seconds']>2
    assert 'requirements' in heard['text'].lower() and 'check' in heard['text'].lower(),heard['text']
    print(json.dumps({'audio_seconds':spoken['seconds'],'synthesis_ms':spoken['duration_ms'],'recognition_ms':heard['duration_ms'],'recognized_test_answer':heard['text']}))
finally:
    worker.stdin.close();worker.wait(timeout=15)
