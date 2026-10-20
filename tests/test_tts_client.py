import contextlib
import importlib.util
import io
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch
import urllib.error
import xml.etree.ElementTree as ET

ROOT = Path(__file__).resolve().parents[1]
spec = importlib.util.spec_from_file_location('tts_client', ROOT / 'skills/common/tts_client.py')
client = importlib.util.module_from_spec(spec)
spec.loader.exec_module(client)


class Response:
    def __init__(self, body=b'ID3fake-mp3', content_type='audio/mpeg'):
        self.body = body
        self.headers = {'Content-Type': content_type}
    def __enter__(self): return self
    def __exit__(self, *args): pass
    def read(self): return self.body


class TTSClientTests(unittest.TestCase):
    def setUp(self):
        self.folder = tempfile.TemporaryDirectory()
        self.addCleanup(self.folder.cleanup)
        self.output = Path(self.folder.name) / 'audio.mp3'

    def generate(self, **kwargs):
        return client.synthesize('private-token', kwargs.pop('channel', 'edge'), '你好 < & "世界"',
                                 'zh-CN-XiaoxiaoNeural', self.output, retry_delay=0, **kwargs)

    def test_retry_then_success_without_secret_output(self):
        errors = [urllib.error.HTTPError('https://tts.cookup.cn/api/ra', 500, 'bad', {}, None),
                  urllib.error.URLError('offline'), Response()]
        stderr = io.StringIO()
        with patch.object(client.urllib.request, 'urlopen', side_effect=errors) as call, contextlib.redirect_stderr(stderr):
            self.generate()
        self.assertEqual(call.call_count, 3)
        self.assertEqual(self.output.read_bytes(), b'ID3fake-mp3')
        self.assertNotIn('private-token', stderr.getvalue())
        request = call.call_args.args[0]
        self.assertEqual(request.get_header('Token'), 'private-token')
        self.assertIn('你好', request.data.decode())

    def test_three_retries_then_stop_and_no_corrupt_output(self):
        with patch.object(client.urllib.request, 'urlopen', side_effect=urllib.error.URLError('offline')) as call:
            with contextlib.redirect_stderr(io.StringIO()), self.assertRaisesRegex(RuntimeError, '4 attempts'):
                self.generate()
        self.assertEqual(call.call_count, 4)
        self.assertFalse(self.output.exists())

    def test_invalid_audio_retried_and_existing_audio_preserved(self):
        self.output.write_bytes(b'existing')
        with patch.object(client.urllib.request, 'urlopen', return_value=Response(b'{"error":"bad"}', 'application/json')) as call:
            with contextlib.redirect_stderr(io.StringIO()), self.assertRaises(RuntimeError):
                self.generate(force=True)
        self.assertEqual(call.call_count, 4)
        self.assertEqual(self.output.read_bytes(), b'existing')

    def test_existing_output_requires_explicit_overwrite(self):
        self.output.write_bytes(b'existing')
        with patch.object(client.urllib.request, 'urlopen') as call, self.assertRaises(ValueError):
            self.generate()
        call.assert_not_called()

    def test_azure_ssml_and_voice_are_escaped(self):
        route, body, headers = client.build_request('azure', '<你好 & "世界">', 'voice"<&', -10, -20, 5)
        self.assertEqual(route, '/api/azure')
        self.assertEqual(headers['Format'], 'audio-24khz-48kbitrate-mono-mp3')
        tree = ET.fromstring(body.decode())
        voice = list(tree)[0]
        self.assertEqual(voice.attrib['name'], 'voice"<&')
        self.assertEqual(list(voice)[0].text, '<你好 & "世界">')
        with patch.object(client.urllib.request, 'urlopen', return_value=Response()):
            self.generate(channel='azure')

    def test_edge_wav_rejected_without_request(self):
        with patch.object(client.urllib.request, 'urlopen') as call, self.assertRaises(ValueError):
            self.generate(format='wav')
        call.assert_not_called()


if __name__ == '__main__':
    unittest.main()
