#!/usr/bin/env python3
"""Exercise extension discovery, approval, host API limits and failure handling with the real executable."""
import json
import os
import pathlib
import shutil
import subprocess
import tempfile

repo = pathlib.Path(__file__).resolve().parent.parent
binary = repo / 'bin/tidecontacts'
with tempfile.TemporaryDirectory(prefix='tidecontacts-ext-') as root:
    exts = pathlib.Path(root) / 'extensions'
    demo = exts / 'relationship-digest'
    demo.mkdir(parents=True)
    shutil.copy(repo / 'examples/extensions/relationship-digest/extension.toml', demo)
    subprocess.run(['go', 'build', '-buildvcs=false', '-o', str(demo / 'relationship-digest'),
                    './examples/extensions/relationship-digest'], cwd=repo, check=True)
    env = dict(os.environ, TERM='dumb', NO_COLOR='1', TIDECONTACTS_EXTENSIONS_DIR=str(exts),
               XDG_CONFIG_HOME=root + '/config', XDG_DATA_HOME=root + '/data',
               XDG_STATE_HOME=root + '/state')

    def command(*args, code=0):
        result = subprocess.run([str(binary), '--demo', *args], env=env, text=True,
                                capture_output=True, check=False, timeout=60)
        assert result.returncode == code, (args, result.returncode, result.stderr)
        assert '\x1b' not in result.stdout + result.stderr
        if code:
            assert not result.stdout
            return json.loads(result.stderr)
        assert not result.stderr, result.stderr
        return json.loads(result.stdout)

    listed = command('extensions', 'list')['data']
    assert [e['id'] for e in listed] == ['relationship-digest'] and listed[0]['status'] == 'disabled'
    assert command('extensions', 'run', 'relationship-digest', 'digest', code=6)['error']['code'] == 'permission_denied'
    assert command('extensions', 'enable', 'relationship-digest', code=6)['error']['code'] == 'approval_required'
    assert command('extensions', 'enable', 'relationship-digest', '--yes')['data']['status'] == 'enabled'
    digest = command('extensions', 'run', 'relationship-digest', 'digest')['data']
    assert digest['title'] == 'Relationship digest' and any(l['label'] == 'People' for l in digest['lines'])
    probe = command('extensions', 'run', 'relationship-digest', 'probe')['data']
    assert 'permission_denied' in probe['lines'][0]['text']
    caps = {c['id']: c for c in command('capabilities', 'list')['data']}
    assert caps['core.contacts']['available'] and caps['relationship-digest.summary']['available']
    command('extensions', 'disable', 'relationship-digest')
    assert not {c['id']: c for c in command('capabilities', 'list')['data']}['relationship-digest.summary']['available']
    # A hostile extension cannot take the host down.
    bad = exts / 'hang-ext'
    bad.mkdir()
    (bad / 'extension.toml').write_text('id = "hang-ext"\nname = "Hang"\nversion = "1.0.0"\napi = 1\ncommand = "run.sh"\n')
    (bad / 'run.sh').write_text('#!/bin/sh\nsleep 120\n')
    (bad / 'run.sh').chmod(0o755)
    assert command('extensions', 'enable', 'hang-ext', '--yes', code=1)['error']['code'] == 'runtime'
    assert {e['id']: e['status'] for e in command('extensions', 'list')['data']}['hang-ext'] == 'disabled'
    assert len(command('contacts', 'list')['data']) == 6
print('PASS: extension discovery, approval, host API, capability discovery and failure isolation')
