#!/usr/bin/env python3
"""Generate fixed Edge/Azure previews on the server; credentials stay in the environment."""
import argparse
import json
import os
from pathlib import Path
import time
import urllib.error
import urllib.request

VOICES = ['zh-CN-XiaoxiaoNeural', 'zh-CN-XiaoyiNeural', 'zh-CN-YunxiNeural',
          'zh-CN-YunjianNeural', 'zh-CN-YunxiaNeural', 'zh-CN-YunyangNeural']
TEXT = '你好，欢迎使用语音合成服务。清晨的阳光洒在窗前，让我们用声音记录每一个美好的瞬间。'
FORMAT = 'audio-24khz-48kbitrate-mono-mp3'


def valid_audio(data):
    return len(data) > 1024 and (data.startswith(b'ID3') or (data[0] == 255 and data[1] & 224 == 224))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--url', default='http://127.0.0.1:1233')
    parser.add_argument('--output', type=Path, required=True)
    parser.add_argument('--force', action='store_true')
    args = parser.parse_args()
    token = os.environ.get('TTS_SAMPLE_TOKEN', '')
    if not token:
        parser.error('Set TTS_SAMPLE_TOKEN to an ordinary user token')
    args.output.mkdir(parents=True, exist_ok=True)
    args.output.chmod(0o755)
    for channel in ('edge', 'azure'):
        folder = args.output / channel
        folder.mkdir(parents=True, exist_ok=True)
        folder.chmod(0o755)
        for voice in VOICES:
            target = folder / (voice + '.mp3')
            if target.exists() and valid_audio(target.read_bytes()) and not args.force:
                print('Reused:', channel, voice, flush=True)
                continue
            headers = {'Token': token}
            if channel == 'edge':
                route = '/api/ra'
                headers['Content-Type'] = 'application/json'
                body = json.dumps({'text': TEXT, 'voice': voice, 'rate': '+0%', 'volume': '+0%',
                                   'pitch': '+0Hz', 'format': FORMAT, 'response_format': 'audio'}).encode()
            else:
                route = '/api/azure'
                headers.update({'Content-Type': 'application/ssml+xml', 'Format': FORMAT})
                body = ('<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="zh-CN">'
                        '<voice name="' + voice + '"><prosody rate="+0%" volume="+0%" pitch="+0Hz">'
                        + TEXT + '</prosody></voice></speak>').encode()
            for attempt in range(3):
                try:
                    request = urllib.request.Request(args.url.rstrip('/') + route, data=body, headers=headers)
                    with urllib.request.urlopen(request, timeout=40) as response:
                        data = response.read()
                        if not response.headers.get('Content-Type', '').startswith('audio/') or not valid_audio(data):
                            raise RuntimeError('Response is not valid MP3 audio')
                    temporary = target.with_suffix('.tmp')
                    temporary.write_bytes(data)
                    temporary.chmod(0o644)
                    temporary.replace(target)
                    print('Generated:', channel, voice, len(data), 'bytes', flush=True)
                    break
                except (urllib.error.URLError, TimeoutError, RuntimeError):
                    if attempt == 2:
                        raise
                    time.sleep(2 ** (attempt + 1))
            time.sleep(1)
    metadata = {'text': TEXT, 'format': FORMAT, 'rate': '+0%', 'volume': '+0%', 'pitch': '+0Hz',
                'channels': ['edge', 'azure'], 'voices': VOICES}
    manifest = args.output / 'manifest.json'
    manifest.write_text(json.dumps(metadata, ensure_ascii=False, indent=2) + '\n')
    manifest.chmod(0o644)


if __name__ == '__main__':
    main()
