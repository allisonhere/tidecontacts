#!/usr/bin/env python3
"""Exercise Phase 2 through a real PTY and verify committed data with SQLite."""
import fcntl
import os
import pathlib
import pty
import select
import sqlite3
import struct
import subprocess
import sys
import tempfile
import termios
import time

binary = str(pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else './tidecontacts').resolve())

with tempfile.TemporaryDirectory(prefix='tidecontacts-crud-') as root:
    env = dict(os.environ, TERM='xterm-256color', NO_COLOR='1',
               XDG_CONFIG_HOME=root + '/config', XDG_DATA_HOME=root + '/data',
               XDG_STATE_HOME=root + '/state')
    master, slave = pty.openpty()
    before = termios.tcgetattr(slave)
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 32, 110, 0, 0))
    process = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave, env=env)
    transcript = bytearray()

    def drain(seconds=0.18):
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

    def send(data, delay=0.18):
        os.write(master, data if isinstance(data, bytes) else data.encode())
        drain(delay)

    def query(sql):
        with sqlite3.connect(root + '/data/tidecontacts/tidecontacts.db') as db:
            return db.execute(sql).fetchall()

    try:
        drain(1)
        send('n')
        send('Alice Offline')
        send(b'\x1b[17~')  # F6: Methods
        send(b'\x0e')  # Ctrl+N: new method
        send(b'\t')
        send('work')
        send(b'\t')
        send('alice@example.com')
        send(b'\x1bOQ', 0.5)  # F2 save
        assert query("SELECT display_name FROM entities WHERE kind='person'") == [('Alice Offline',)], bytes(transcript[-4000:])
        assert query("SELECT value FROM methods WHERE kind='email'") == [('alice@example.com',)]
        send('e')
        for _ in range(5):
            send(b'\x1b[17~')
        send('First line')
        send(b'\r')
        send('Second line')
        send(b'\x1bOQ', 0.5)
        assert query("SELECT notes FROM entities WHERE kind='person'") == [('First line\nSecond line',)]
        send('e')
        for _ in range(2):
            send(b'\x1b[17~')
        send(b'\x0e')
        send('Riverside')
        send(b'\t')
        send('Engineer')
        send(b'\x1bOQ', 0.5)
        assert query("SELECT title FROM memberships") == [('Engineer',)]
        send('f', 0.4)
        assert query("SELECT favorite FROM entities WHERE kind='person'") == [(1,)]
        send('a', 0.4)
        assert query("SELECT archived FROM entities WHERE kind='person'") == [(1,)]
        send('u', 0.4)
        assert query("SELECT archived FROM entities WHERE kind='person'") == [(0,)]
        destination = root + '/contacts.vcf'
        send('E')
        send(destination)
        send(b'\r')
        send('y', 0.5)
        assert pathlib.Path(destination).exists(), bytes(transcript[-4000:])
        send(b'\x1b')
        send('I')
        send(destination)
        send(b'\r', 0.6)
        assert b'Dry run' in transcript, bytes(transcript[-4000:])
        send(' ')  # Explicitly include duplicate as a separate new UUID.
        send(b'\r')
        send('y', 0.6)
        assert query("SELECT COUNT(*) FROM entities WHERE kind='person'") == [(2,)], bytes(transcript[-5000:])
        send(b'\x1b')
        send('q', 0.5)
        process.wait(timeout=8)
        assert process.returncode == 0
        assert termios.tcgetattr(slave) == before
        # Reopen through normal initialization, with no migration/data loss.
        subprocess.run([binary, '--check'], env=env, check=True, capture_output=True, timeout=10)
        assert query("SELECT COUNT(*) FROM entities WHERE kind='person'") == [(2,)]
    finally:
        if process.poll() is None:
            process.kill()
            process.wait()
        os.close(master)
        os.close(slave)
print('PASS: real TUI create, email, multiline notes, membership, favorite/archive/undo, export, duplicate-reviewed import, reopen')
