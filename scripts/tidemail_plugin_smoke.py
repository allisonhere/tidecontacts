#!/usr/bin/env python3
"""Real-PTY contacts bridge check using only a temporary fictional profile."""
import fcntl
import os
import pathlib
import pty
import select
import shutil
import signal
import sqlite3
import struct
import subprocess
import tempfile
import termios
import time

project = pathlib.Path(__file__).resolve().parent.parent
with tempfile.TemporaryDirectory(prefix='tidecontacts-plugin-') as tmp:
    root = pathlib.Path(tmp)
    env = dict(os.environ, TERM='xterm-256color', NO_COLOR='1', TIDECONTACTS_DEMO='1',
               XDG_CONFIG_HOME=str(root / 'config'), XDG_DATA_HOME=str(root / 'data'),
               XDG_STATE_HOME=str(root / 'state'))
    subprocess.run([str(project / 'bin/tidemail-demo-seed')], env=env, check=True)
    subprocess.run([str(project / 'bin/tidecontacts'), '--demo', '--check'], env=env, check=True,
                   stdout=subprocess.DEVNULL)
    destination = root / 'config/tidemail/plugins/tidecontacts'
    shutil.copytree(project / 'bin/plugins/tidecontacts', destination)
    master, slave = pty.openpty()
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 120, 0, 0))
    proc = subprocess.Popen([str(project / 'bin/tidemail')], env=env, stdin=slave, stdout=slave, stderr=slave)
    transcript = bytearray()

    def drain(seconds=0.4):
        until = time.monotonic() + seconds
        while time.monotonic() < until:
            if select.select([master], [], [], max(0, until - time.monotonic()))[0]:
                try:
                    chunk = os.read(master, 65536)
                except OSError:
                    break
                transcript.extend(chunk)
                if b'\x1b]11;' in chunk:
                    os.write(master, b'\x1b]11;rgb:0000/0000/0000\x1b\\')
                if b'\x1b[6n' in chunk:
                    os.write(master, b'\x1b[1;1R')

    def send(value, seconds=0.4):
        os.write(master, value.encode())
        drain(seconds)

    try:
        drain(2)
        send('C', 1)
        assert b'Recent correspondence' in transcript, bytes(transcript[-5000:])
        assert b'person1@example.com' in transcript, bytes(transcript[-5000:])
        start = len(transcript)
        send('A', 2)
        assert b'TideContacts /' in transcript[start:], bytes(transcript[start:][-6000:])
        send('q', 1)
        assert proc.poll() is None, 'TideMail did not resume after Advanced CRM'
        send('n')
        send('Bridge Test')
        send('\t')
        send('bridge-test@example.com')
        send('\r', 1)
        database = root / 'data/tidecontacts/demo/tidecontacts.db'
        with sqlite3.connect(database) as db:
            assert db.execute("SELECT count(*) FROM methods WHERE value='bridge-test@example.com'").fetchone()[0] == 1, bytes(transcript[-10000:])
        send('/')
        send('Bridge Test')
        send('\r', 1)
        send('c', 1)
        assert b'bridge-test@example.com' in transcript, bytes(transcript[-5000:])
        assert not (root / 'data/tidecontacts/tidecontacts.db').exists(), 'demo opened regular contacts database'
        print('PASS: plugin discovery, Advanced CRM launch/return, demo people, add/save, search, compose, isolated data')
    finally:
        proc.send_signal(signal.SIGTERM)
        try:
            proc.wait(timeout=5)
        except subprocess.TimeoutExpired:
            proc.kill()
            proc.wait()
        os.close(master)
        os.close(slave)
