"""Disposable ElevenLabs-compatible service for CI; never uses a real API key."""
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
import io
import json
import time
import threading
import wave
import math
import struct
from email.parser import BytesParser
from email.policy import default

lock = threading.Lock()
state = {'calls': 0, 'isolations': 0, 'lists': 0, 'fail_next': False, 'delay': 0, 'extra_voice': False}
audio = io.BytesIO()
with wave.open(audio, 'wb') as wav:
    wav.setnchannels(1); wav.setsampwidth(2); wav.setframerate(16000)
    # Provider output with dead air at both ends for processed-duration coverage.
    wav.writeframes(b''.join(struct.pack('<h', int(2000*math.sin(2*math.pi*700*i/16000)) if 16000 <= i < 24000 else 0) for i in range(48000)))

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
        isolation = self.path == '/v1/audio-isolation'
        if isolation or self.path.startswith('/v1/speech-to-speech/'):
            if b'RIFF' not in body: return self.reply(422, {})
            if isolation:
                if b'name="model_id"' in body or b'name="voice_settings"' in body or b'name="file_format"' not in body: return self.reply(422, {})
                message=BytesParser(policy=default).parsebytes(('Content-Type: '+self.headers['Content-Type']+'\r\nMIME-Version: 1.0\r\n\r\n').encode()+body)
                uploaded=next(p.get_payload(decode=True) for p in message.iter_parts() if p.get_param('name',header='content-disposition')=='audio')
                with wave.open(io.BytesIO(uploaded),'rb') as wav:
                    if wav.getnframes()/wav.getframerate()<4.6: return self.reply(400,{'detail':{'status':'invalid_audio_duration','message':'Minimum duration is 4.6 seconds'}})
            elif b'eleven_multilingual_sts_v2' not in body: return self.reply(422, {})
            with lock:
                state['calls'] += 1
                if isolation: state['isolations'] += 1
                fail, delay = state['fail_next'], state['delay']
                state['fail_next'] = False
            time.sleep(delay)
            if fail: return self.reply(429, {})
            return self.reply(200, uploaded if isolation else audio.getvalue(), 'audio/mpeg')
        self.reply(404, {})

ThreadingHTTPServer(('0.0.0.0', 8080), Handler).serve_forever()
