#!/usr/bin/env python3
"""Launch the contacts integration with persistent, isolated fictional data."""
import os
import pathlib
import shutil
import subprocess

project = pathlib.Path(__file__).resolve().parent.parent
root = project / 'bin' / 'tidemail-demo'
env = dict(os.environ, TIDECONTACTS_DEMO='1',
           XDG_CONFIG_HOME=str(root / 'config'),
           XDG_DATA_HOME=str(root / 'data'), XDG_STATE_HOME=str(root / 'state'))
for binary in ('tidemail', 'tidemail-demo-seed', 'tidecontacts'):
    if not (project / 'bin' / binary).is_file():
        raise SystemExit('Build the integration first: make tidemail-demo-build')
subprocess.run([str(project / 'bin' / 'tidemail-demo-seed')], env=env, check=True)
subprocess.run([str(project / 'bin' / 'tidecontacts'), '--demo', '--check'], env=env, check=True,
               stdout=subprocess.DEVNULL)
plugin = root / 'config' / 'tidemail' / 'plugins' / 'tidecontacts'
plugin.mkdir(parents=True, exist_ok=True)
for name in ('plugin.toml', 'tidecontacts-plugin', 'tidecontacts'):
    shutil.copy2(project / 'bin' / 'plugins' / 'tidecontacts' / name, plugin / name)
raise SystemExit(subprocess.run([str(project / 'bin' / 'tidemail')], env=env).returncode)
