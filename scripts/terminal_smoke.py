#!/usr/bin/env python3
"""Linux/macOS PTY integration smoke test; all application data is temporary."""
import fcntl
import os
import pathlib
import pty
import select
import signal
import struct
import subprocess
import sys
import tempfile
import termios
import time

binary = str(pathlib.Path(sys.argv[1] if len(sys.argv) > 1 else './tidecontacts').resolve())


def drain(fd, duration=0.3):
    output = b''
    end = time.monotonic() + duration
    while time.monotonic() < end:
        if select.select([fd], [], [], max(0, end - time.monotonic()))[0]:
            try:
                chunk = os.read(fd, 65536)
            except OSError:
                break
            output += chunk
            if b'\x1b]11;' in chunk:
                os.write(fd, b'\x1b]11;rgb:0000/0000/0000\x1b\\')
            if b'\x1b[6n' in chunk:
                os.write(fd, b'\x1b[1;1R')
    return output


with tempfile.TemporaryDirectory(prefix='tidecontacts-smoke-') as root:
    env = dict(os.environ, TERM='xterm-256color', NO_COLOR='1',
               XDG_CONFIG_HOME=root + '/config', XDG_DATA_HOME=root + '/data',
               XDG_STATE_HOME=root + '/state')
    for _ in range(2):
        check = subprocess.run([binary, '--check'], env=env, capture_output=True, timeout=10)
        assert check.returncode == 0, check.stderr
        assert b'SQLite OK' in check.stdout
    for ending in ('q', 'ctrl+c', 'signal', 'hangup'):
        master, slave = pty.openpty()
        before = termios.tcgetattr(slave)
        fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 32, 110, 0, 0))
        process = subprocess.Popen([binary], stdin=slave, stdout=slave, stderr=slave, env=env)
        try:
            transcript = drain(master, 0.8)
            assert b'TideContacts' in transcript, transcript
            for keys in (b'8', b'/', b'q1j?:', b'\x1b', b':', b'light', b'\r'):
                os.write(master, keys)
                transcript += drain(master)
            fcntl.ioctl(slave, termios.TIOCSWINSZ, struct.pack('HHHH', 20, 60, 0, 0))
            process.send_signal(signal.SIGWINCH)
            transcript += drain(master)
            if ending in ('signal', 'hangup'):
                process.send_signal(signal.SIGTERM if ending == 'signal' else signal.SIGHUP)
            else:
                os.write(master, b'q' if ending == 'q' else b'\x03')
            transcript += drain(master, 0.5)
            process.wait(timeout=8)
            assert process.returncode == 0, (ending, process.returncode, transcript)
            assert termios.tcgetattr(slave) == before, ending + ': terminal modes not restored'
            assert b'\x1b[?1049l' in transcript, ending + ': alternate screen not restored'
        finally:
            if process.poll() is None:
                process.kill()
                process.wait()
            os.close(master)
            os.close(slave)
    # Invalid configuration must fail without modifying the file or opening raw mode.
    config = pathlib.Path(root) / 'config/tidecontacts/config.toml'
    config.parent.mkdir(parents=True, exist_ok=True)
    config.write_text('theme = "invalid"\n')
    failed = subprocess.run([binary, '--check'], env=env, capture_output=True, timeout=10)
    assert failed.returncode != 0 and b'theme must be' in failed.stderr
    assert config.read_text() == 'theme = "invalid"\n'
print('PASS: initialize/reopen, PTY navigation/input/palette/resize, q/Ctrl+C/SIGTERM/SIGHUP restore, invalid config')
