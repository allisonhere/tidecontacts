#!/usr/bin/env python3
"""Exercise the Phase 3 CRM through a real PTY and verify committed data with SQLite."""
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

with tempfile.TemporaryDirectory(prefix='tidecontacts-crm-') as root:
    env = dict(os.environ, TERM='xterm-256color', NO_COLOR='1',
               XDG_CONFIG_HOME=root + '/config', XDG_DATA_HOME=root + '/data',
               XDG_STATE_HOME=root + '/state')
    master, slave = pty.openpty()
    before = termios.tcgetattr(slave)
    fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 150, 0, 0))
    process = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave, env=env)
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

    def send(data, delay=0.25):
        os.write(master, data if isinstance(data, bytes) else data.encode())
        drain(delay)

    def query(sql):
        with sqlite3.connect(root + '/data/tidecontacts/tidecontacts.db') as db:
            return db.execute(sql).fetchall()

    def tail():
        return bytes(transcript[-6000:])

    F2 = b'\x1bOQ'
    try:
        drain(1)
        send('n')
        send('Alice Offline')
        send(F2, 0.5)
        assert query("SELECT display_name FROM entities") == [('Alice Offline',)], tail()

        # Quick capture from the contact: an activity and a task, both linked automatically.
        send('A')
        send('Coffee and roadmap type:meeting on:yesterday at:10:30')
        send(b'\r', 0.5)
        assert query("SELECT type,title FROM activities") == [('meeting', 'Coffee and roadmap')], tail()
        assert query("SELECT count(*) FROM activity_entities") == [(1,)]
        send('T')
        send('Send notes due:today pri:high every:week')
        send(b'\r', 0.5)
        assert query("SELECT title,status,priority FROM tasks") == [('Send notes', 'scheduled', 3)], tail()
        assert query("SELECT freq FROM recurrence_rules") == [('weekly',)]
        send('F')
        send('monthly')
        send(b'\r', 0.5)
        assert query("SELECT every,unit FROM followup_prefs") == [(1, 'month')], tail()

        # The dossier shows it all; Enter follows the task and Esc returns to the person.
        send('[', 0.3)
        send('[', 0.4)
        assert b'Open tasks (1)' in transcript and b'Follow-up' in transcript, tail()
        send(b'\r', 0.5)
        assert b'TASKS' in transcript, tail()
        send('c', 0.6)  # complete: logs a timeline entry and creates exactly one successor
        assert query("SELECT status,occurrence FROM tasks ORDER BY occurrence") == [('completed', 0), ('scheduled', 1)], tail()
        assert query("SELECT count(*) FROM activities WHERE type='task-completion'") == [(1,)]
        send(b'\x1b', 0.5)
        assert b'Dossier' in transcript, tail()

        # A project through the form, and a pipeline driven entirely from the keyboard.
        send('6', 0.4)
        send('n')
        send('Harbor')
        send(F2, 0.5)
        assert query("SELECT name,status FROM projects") == [('Harbor', 'active')], tail()
        send('7', 0.4)
        send('n')
        send('Applications')
        send(F2, 0.6)
        assert query("SELECT name FROM pipeline_stages ORDER BY position") == [('To do',), ('Doing',), ('Done',)], tail()
        send('n')
        send('Riverside Studio')
        send(F2, 0.6)
        send('>', 0.5)
        send('>', 0.5)
        assert query("SELECT g.name FROM pipeline_items i JOIN pipeline_stages g ON g.id=i.stage_id") == [('Done',)], tail()
        assert query("SELECT count(*) FROM pipeline_stage_history") == [(3,)]

        # Resize to a narrow terminal: the app stays alive and redraws the stage list.
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 18, 50, 0, 0))
        process.send_signal(28)  # SIGWINCH
        drain(0.5)
        assert process.poll() is None, tail()
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 40, 150, 0, 0))
        process.send_signal(28)
        drain(0.4)

        # People Inbox lists the reminder once it is due.
        send('b')
        send('Stretch at:00:00')
        send(b'\r', 0.6)
        assert query("SELECT title,status FROM reminders") == [('Stretch', 'pending')], tail()
        send('8', 0.6)
        assert b'Stretch' in transcript and b'Why this is here' in transcript, tail()
        due = subprocess.run([binary, '--due'], env=env, capture_output=True, timeout=10)
        assert due.returncode == 3 and b'Stretch' in due.stdout, due
        send('x', 0.6)  # dismiss from the inbox... which dismisses the reminder itself
        send('q', 0.5)
        process.wait(timeout=8)
        assert process.returncode == 0
        assert termios.tcgetattr(slave) == before
        subprocess.run([binary, '--check'], env=env, check=True, capture_output=True, timeout=10)
        assert query("SELECT max(version) FROM schema_migrations") == [(3,)]
        assert query("PRAGMA foreign_key_check") == []
        assert query("SELECT count(*) FROM entities") == [(1,)]
        mode = os.stat(root + '/data/tidecontacts/tidecontacts.db').st_mode & 0o777
        assert mode == 0o600, oct(mode)
        log = pathlib.Path(root + '/state/tidecontacts/tidecontacts.log').read_text()
        assert 'Alice' not in log and 'Coffee' not in log, 'personal data in log'
    finally:
        if process.poll() is None:
            process.kill()
        os.close(master)
        os.close(slave)
print('PASS: real TUI capture, dossier, back-navigation, recurrence, project, keyboard pipeline, resize, reminder, inbox, --due, reopen')
