#!/usr/bin/env python3
"""Generate audio using tts.cookup.cn. Supports Edge and Azure; retries failures 3 times."""
import argparse
import getpass
import http.client
import json
import os
from pathlib import Path
import ssl
import sys
import tempfile
import time
import urllib.error
import urllib.request
from xml.sax.saxutils import escape, quoteattr

BASE_URL = 'https://tts.cookup.cn'
FORMATS = {'mp3': ('audio-24khz-48kbitrate-mono-mp3', 'audio/mpeg'),
           'webm': ('webm-24khz-16bit-mono-opus', 'audio/webm'),
           'wav': ('riff-24khz-16bit-mono-pcm', 'audio/wav')}


def signed(value, unit):
    return f'{value:+d}{unit}'


def build_request(channel, text, voice, rate=0, volume=0, pitch=0, format='mp3'):
    audio_format = FORMATS[format][0]
    rate, volume, pitch = signed(rate, '%'), signed(volume, '%'), signed(pitch, 'Hz')
    if channel == 'edge':
        payload = dict(text=text, voice=voice, rate=rate, volume=volume, pitch=pitch,
                       format=audio_format, response_format='audio')
        return '/api/ra', json.dumps(payload, ensure_ascii=False).encode('utf-8'), {'Content-Type': 'application/json'}
    ssml = ('<speak version="1.0" xmlns="http://www.w3.org/2001/10/synthesis" xml:lang="zh-CN">'
            '<voice name=' + quoteattr(voice) + '><prosody rate=' + quoteattr(rate) + ' volume=' + quoteattr(volume)
            + ' pitch=' + quoteattr(pitch) + '>' + escape(text) + '</prosody></voice></speak>')
    return '/api/azure', ssml.encode('utf-8'), {'Content-Type': 'application/ssml+xml', 'Format': audio_format}


def valid_audio(data, format):
    if format == 'mp3':
        return len(data) > 2 and (data.startswith(b'ID3') or data[0] == 255 and data[1] & 224 == 224)
    if format == 'webm':
        return data.startswith(b'\x1a\x45\xdf\xa3')
    return len(data) > 12 and data.startswith(b'RIFF') and data[8:12] == b'WAVE'


def synthesize(token, channel, text, voice, output, rate=0, volume=0, pitch=0,
               format='mp3', force=False, base_url=BASE_URL, retry_delay=2, sleep=time.sleep):
    output = Path(output).expanduser().resolve()
    if output.exists() and not force:
        raise ValueError('Output exists; choose a new filename or explicitly use --force')
    if not text.strip():
        raise ValueError('Text must not be empty')
    if channel == 'edge' and format == 'wav':
        raise ValueError('Edge supports mp3/webm; choose Azure for WAV')
    route, body, headers = build_request(channel, text, voice, rate, volume, pitch, format)
    headers['Token'] = token
    last_error = 'Unknown error'
    data = None
    for attempt in range(4):  # first attempt + 3 retries, no unbounded agent retry loop
        try:
            request = urllib.request.Request(base_url.rstrip('/') + route, data=body, headers=headers)
            with urllib.request.urlopen(request, timeout=40) as response:
                candidate = response.read()
                content_type = response.headers.get('Content-Type', '').split(';', 1)[0].strip()
                if not content_type.startswith('audio/') or not valid_audio(candidate, format):
                    raise ValueError('Server did not return valid audio (possibly a JSON/text error)')
                data = candidate
            break
        except urllib.error.HTTPError as error:
            # Do not echo credentials, request bodies, or arbitrary server text.
            last_error = 'HTTP ' + str(error.code)
        except (urllib.error.URLError, TimeoutError, OSError, ValueError, ssl.SSLError, http.client.HTTPException) as error:
            last_error = type(error).__name__ + ': request failed or audio response was invalid'
        if attempt < 3:
            print(f'Attempt {attempt + 1} failed ({last_error}); retry {attempt + 1}/3.', file=sys.stderr)
            sleep(retry_delay * (2 ** attempt))
    if data is None:
        raise RuntimeError('Failed after 4 attempts (3 retries): ' + last_error)
    # Only complete, validated audio is installed as output. Existing files stay intact on failure.
    fd, temporary = tempfile.mkstemp(prefix='.tts-audio-', suffix='.' + format, dir=output.parent)
    try:
        with os.fdopen(fd, 'wb') as file:
            file.write(data)
            file.flush()
            os.fsync(file.fileno())
        if force:
            os.replace(temporary, output)
        else:
            os.link(temporary, output)
    finally:
        Path(temporary).unlink(missing_ok=True)
    return output


def ranged(low, high):
    def validate(value):
        result = int(value)
        if not low <= result <= high:
            raise argparse.ArgumentTypeError(f'expected {low}..{high}')
        return result
    return validate


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__)
    p.add_argument('--interactive', action='store_true', help='prompt for token first, then basic parameters')
    p.add_argument('--channel', choices=['edge', 'azure'], default='edge')
    p.add_argument('--voice', default='zh-CN-XiaoxiaoNeural')
    inputs = p.add_mutually_exclusive_group()
    inputs.add_argument('--text')
    inputs.add_argument('--text-file', type=Path, help='UTF-8 plain text file')
    p.add_argument('--rate', type=ranged(-50, 100), default=0)
    p.add_argument('--volume', type=ranged(-100, 0), default=0)
    p.add_argument('--pitch', type=ranged(-50, 50), default=0)
    p.add_argument('--format', choices=list(FORMATS), default='mp3')
    p.add_argument('--output', type=Path)
    p.add_argument('--force', action='store_true', help='explicitly replace an existing output on success')
    args = p.parse_args(argv)
    try:
        token = os.environ.get('TTS_TOKEN', '').strip()
        if not token:
            if not sys.stdin.isatty():
                raise ValueError('Set TTS_TOKEN privately or run from an interactive terminal to enter it hidden')
            token = getpass.getpass('普通用户 Token（输入不显示，不是管理 Token）: ').strip()
        if not token:
            raise ValueError('A user Token is required')
        if args.interactive:
            args.channel = input(f'渠道 edge/azure [{args.channel}]: ').strip() or args.channel
            args.voice = input(f'音色 [{args.voice}]: ').strip() or args.voice
            args.rate = ranged(-50, 100)(input(f'语速 -50..100 [{args.rate}]: ').strip() or str(args.rate))
            args.volume = ranged(-100, 0)(input(f'音量 -100..0 [{args.volume}]: ').strip() or str(args.volume))
            args.pitch = ranged(-50, 50)(input(f'音高 Hz -50..50 [{args.pitch}]: ').strip() or str(args.pitch))
            args.format = input(f'格式 mp3/webm/wav [{args.format}]: ').strip() or args.format
            if args.text is None and args.text_file is None:
                value = input('合成文本（或 @UTF-8文件路径）: ')
                if value.startswith('@'):
                    args.text_file = Path(value[1:]).expanduser()
                else:
                    args.text = value
            args.output = Path(input(f'输出文件 [{args.output or ("audio." + args.format)}]: ').strip() or str(args.output or ('audio.' + args.format)))
        if args.channel not in ('edge', 'azure') or args.format not in FORMATS:
            raise ValueError('Invalid channel or format')
        if not args.voice.strip():
            raise ValueError('Voice must not be empty')
        text = args.text_file.expanduser().read_text(encoding='utf-8-sig') if args.text_file else args.text
        if text is None:
            raise ValueError('Provide --text or --text-file, or use --interactive')
        output = synthesize(token, args.channel, text, args.voice, args.output or ('audio.' + args.format),
                            args.rate, args.volume, args.pitch, args.format, args.force)
        print('Audio saved: ' + str(output))
        return 0
    except (ValueError, OSError, RuntimeError, EOFError, argparse.ArgumentTypeError) as error:
        print(str(error), file=sys.stderr)
        return 1


if __name__ == '__main__':
    raise SystemExit(main())
