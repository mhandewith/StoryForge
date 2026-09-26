"""Disposable ElevenLabs-compatible service for CI; never uses a real API key."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import time
import threading
import wave

lock = threading.Lock()
state = {'calls': 0, 'lists': 0, 'fail_next': False, 'delay': 0, 'extra_voice': False}
audio = io.BytesIO()
with wave.open(audio, 'wb') as wav:
    wav.setnchannels(1); wav.setsampwidth(2); wav.setframerate(16000)
    wav.writeframes(b'\x20\x00' * 16000)

class Handler(BaseHTTPRequestHandler):
    def log_message(self, *args): pass
    def reply(self, code, body, content='application/json'):
        if not isinstance(body, bytes): body = json.dumps(body).encode()
        self.send_response(code); self.send_header('Content-Type', content)
        self.send_header('Content-Length', str(len(body))); self.end_headers()
        self.wfile.write(body)
    def do_GET(self):
        if self.path == '/test/state':
            with lock: result = dict(state)
            return self.reply(200, result)
        if self.headers.get('xi-api-key') != 'test-key': return self.reply(401, {})
        if self.path.startswith('/v2/voices'):
            with lock:
                state['lists'] += 1
                voices = [{'voice_id': 'voice-wolf', 'name': 'Wolf'}, {'voice_id': 'voice-owl', 'name': 'Owl'}]
                if state['extra_voice']: voices.append({'voice_id': 'voice-new', 'name': 'New voice'})
            return self.reply(200, {'voices': voices, 'has_more': False})
        self.reply(404, {})
    def do_POST(self):
        body = self.rfile.read(int(self.headers.get('Content-Length', 0)))
        if self.path == '/test/control':
            with lock: state.update(json.loads(body))
            return self.reply(200, {})
        if self.headers.get('xi-api-key') != 'test-key': return self.reply(401, {})
        if self.path.startswith('/v1/speech-to-speech/'):
            if b'eleven_multilingual_sts_v2' not in body or b'RIFF' not in body: return self.reply(422, {})
            with lock:
                state['calls'] += 1
                fail, delay = state['fail_next'], state['delay']
                state['fail_next'] = False
            time.sleep(delay)
            if fail: return self.reply(429, {})
            return self.reply(200, audio.getvalue(), 'audio/mpeg')
        self.reply(404, {})

ThreadingHTTPServer(('0.0.0.0', 8080), Handler).serve_forever()
