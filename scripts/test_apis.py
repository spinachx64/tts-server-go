#!/usr/bin/env python3
"""Probe the running service; save responses and a machine-readable report."""
import argparse
import base64
import json
import pathlib
import urllib.error
import urllib.request
import time

p = argparse.ArgumentParser()
p.add_argument('--url', default='http://localhost:1233')
p.add_argument('--token', default='')
p.add_argument('--output', default='output/api-test')
p.add_argument('--extended', action='store_true', help='Also test file input, prosody, binary output, and long text')
p.add_argument('--edge-protocol', choices=['json', 'ssml'], default='json')
a = p.parse_args()
out = pathlib.Path(a.output)
out.mkdir(parents=True, exist_ok=True)
ssml = '<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="zh-CN"><voice name="zh-CN-XiaoxiaoNeural"><prosody rate="0%" pitch="+0Hz">这是接口测试。</prosody></voice></speak>'
cases = [
    ('edge', '/api/ra', {'text': '这是接口测试。', 'voice': 'zh-CN-XiaoxiaoNeural', 'write_subtitles': True} if a.edge_protocol == 'json' else ssml),
    ('azure', '/api/azure', ssml),
    ('creation', '/api/creation', {'text': '这是接口测试。', 'voiceName': 'zh-CN-XiaoxiaoNeural', 'format': 'audio-24khz-48kbitrate-mono-mp3', 'rate': '0%', 'volume': '0%', 'style': 'general', 'styleDegree': '1', 'role': 'default'}),
    ('azure-voices', '/api/azure/voices', None),
    ('creation-voices', '/api/creation/voices', None),
]
if a.edge_protocol == 'json':
    cases.append(('edge-voices', '/api/ra', {'list_voices': True}))
    if a.extended:
        cases.extend([
            ('edge-file', '/api/ra', {'file': {'name': 'test.txt', 'content': '你好，文件输入测试。'}, 'voice': 'zh-CN-XiaoxiaoNeural', 'rate': '-10%', 'volume': '-20%', 'pitch': '+5Hz', 'write_media': 'file.mp3', 'write_subtitles': 'file.srt'}),
            ('edge-audio', '/api/ra', {'text': '音频响应测试。', 'voice': 'zh-CN-XiaoxiaoNeural', 'response_format': 'audio'}),
            ('edge-long', '/api/ra', {'text': '这是长文本测试。' * 160, 'voice': 'zh-CN-XiaoxiaoNeural', 'rate': '+100%', 'write_subtitles': True}),
        ])
report = []
for name, path, payload in cases:
    headers = {'Token': a.token, 'Format': 'audio-24khz-48kbitrate-mono-mp3'}
    headers['Content-Type'] = 'application/json' if isinstance(payload, dict) else 'application/ssml+xml'
    data = json.dumps(payload, ensure_ascii=False).encode() if isinstance(payload, dict) else payload.encode() if payload else None
    req = urllib.request.Request(a.url.rstrip('/') + path, data=data, headers=headers)
    start = time.monotonic()
    status, body, error = None, b'', None
    try:
        with urllib.request.urlopen(req, timeout=40) as response:
            status, body = response.status, response.read()
    except urllib.error.HTTPError as e:
        status, body = e.code, e.read()
    except Exception as e:
        error = str(e)
    (out / (name + '.response')).write_bytes(body)
    ok = False
    if status == 200:
        if name.endswith('voices'):
            try:
                voices = json.loads(body)
                if isinstance(voices, dict):
                    voices = voices.get('voices')
                ok = isinstance(voices, list) and len(voices) > 0
            except (ValueError, TypeError):
                pass
        else:
            audio = body
            result = {}
            if name.startswith('edge') and a.edge_protocol == 'json' and payload.get('response_format') != 'audio':
                try:
                    result = json.loads(body)
                    audio = base64.b64decode(result['audio'], validate=True)
                    (out / (name + '.srt')).write_text(result.get('subtitles', ''), encoding='utf-8')
                except (ValueError, KeyError, TypeError):
                    audio = b''
            if name.startswith('edge') and isinstance(payload, dict) and payload.get('write_subtitles') and not result.get('subtitles'):
                audio = b''
            ok = len(audio) > 100 and (audio.startswith(b'ID3') or audio[0] == 255)
            if ok:
                (out / (name + '.mp3')).write_bytes(audio)
    item = {'case': name, 'api': path, 'ok': ok, 'status': status, 'seconds': round(time.monotonic()-start, 2), 'bytes': len(body)}
    if error:
        item['error'] = error
    elif not ok:
        item['error'] = body.decode(errors='replace')[:500]
    report.append(item)
    print(json.dumps(item, ensure_ascii=False), flush=True)
(out / 'report.json').write_text(json.dumps(report, ensure_ascii=False, indent=2), encoding='utf-8')
raise SystemExit(0 if all(item['ok'] for item in report) else 1)
