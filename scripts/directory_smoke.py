#!/usr/bin/env python3
"""Connect a folder through the real TUI, then refresh through the plugin alone."""
import fcntl
import json
import os
import pathlib
import pty
import select
import sqlite3
import struct
import subprocess
import tempfile
import termios
import time

project = pathlib.Path(__file__).resolve().parent.parent
binary = project / 'bin/tidecontacts'
plugin = project / 'bin/plugins/tidecontacts/tidecontacts-plugin'
with tempfile.TemporaryDirectory(prefix='tidecontacts-directory-') as root:
    env = dict(os.environ, TERM='xterm-256color', NO_COLOR='1',
               XDG_CONFIG_HOME=root + '/config', XDG_DATA_HOME=root + '/data',
               XDG_STATE_HOME=root + '/state')
    folder = pathlib.Path(root) / 'address-book'
    folder.mkdir()
    card = folder / 'one.vcf'

    def write(name):
        card.write_text('BEGIN:VCARD\nVERSION:3.0\nUID:smoke-one\nFN:' + name +
                        '\nEMAIL:one@example.org\nEMAIL:two@example.org\nEND:VCARD\n')

    write('Folder Person')
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 32, 120, 0, 0))
    process = subprocess.Popen([str(binary)], stdin=slave, stdout=slave, stderr=slave, env=env)
    transcript = bytearray()

    def drain(seconds=0.2):
        end = time.monotonic() + seconds
        while time.monotonic() < end:
            if select.select([master], [], [], max(0, end - time.monotonic()))[0]:
                try:
                    data = os.read(master, 65536)
                except OSError:
                    return
                transcript.extend(data)
                if b'\x1b]11;' in data:
                    os.write(master, b'\x1b]11;rgb:0000/0000/0000\x1b\\')
                if b'\x1b[6n' in data:
                    os.write(master, b'\x1b[1;1R')

    def send(data, delay=0.2):
        os.write(master, data if isinstance(data, bytes) else data.encode())
        drain(delay)

    def rows(sql):
        with sqlite3.connect(root + '/data/tidecontacts/tidecontacts.db') as database:
            return database.execute(sql).fetchall()

    def call(method, data):
        request = dict(api=1, type='request', request_id='smoke', method=method, data=data)
        result = subprocess.run([str(plugin)], input=json.dumps(request), text=True,
                                capture_output=True, env=env, timeout=15, check=True)
        response = json.loads(result.stdout)
        assert response['ok'], response
        return response['data']

    try:
        drain(0.7)
        send('I')
        for _ in range(4):
            send(b'\x1b[B', 0.04)
        send(b'\r')
        assert b'Connected vCard folders' in transcript, bytes(transcript[-3000:])
        send('p')
        send(str(folder))
        send(b'\r', 0.4)
        assert rows("SELECT count(*) FROM entities") == [(0,)], 'Preview wrote contacts'
        send(b'\r')
        send('y', 0.5)
        assert rows("SELECT display_name FROM entities WHERE kind='person' AND deleted=0") == [('Folder Person',)]
        send(b'\x1b')
        send('q', 0.3)
        process.wait(timeout=5)
        assert process.returncode == 0
        # No TUI process is running. The plugin must discover the source changes itself.
        write('Changed Folder Person')
        matches = call('contacts.search', {'query': 'Changed Folder'})['contacts']
        assert {c['addr'] for c in matches} == {'one@example.org', 'two@example.org'}, matches
        listed = call('contacts.list', {'query': 'Changed'})['contacts']
        assert len(listed) == 1 and listed[0]['read_only'], listed
        card.rename(folder / 'renamed.vcf')
        assert call('contacts.list', {'query': 'Changed'})['contacts'][0]['id'] == listed[0]['id']
        # A broken source preserves cached results and reports the error.
        (folder / 'broken.vcf').write_text('not a vCard')
        result = call('contacts.list', {'query': 'Changed'})
        assert result['warning'] and result['contacts'], result
        (folder / 'broken.vcf').unlink()
        (folder / 'renamed.vcf').unlink()
        assert call('contacts.list', {'query': 'Changed'})['contacts'] == []
        print('PASS: modal folder connect, preview, plugin-only refresh, all emails, stable UID, read-only marker, failure recovery and removal')
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
        os.close(master)
        os.close(slave)
