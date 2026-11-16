#!/usr/bin/env python3
"""Check the client wire formats and shell invocation with isolated demo data."""
import json
import os
import pathlib
import shlex
import subprocess
import tempfile

project = pathlib.Path(__file__).resolve().parent.parent
binary = project / 'bin/tidecontacts'
with tempfile.TemporaryDirectory(prefix='tidecontacts-mail-') as tmp:
    env = dict(os.environ, TERM='dumb', NO_COLOR='1',
               XDG_CONFIG_HOME=tmp + '/config', XDG_DATA_HOME=tmp + '/data',
               XDG_STATE_HOME=tmp + '/state')
    subprocess.run([str(binary), '--demo', '--check'], env=env, check=True, capture_output=True)

    def lookup(client, query=''):
        return subprocess.run([str(binary), '--demo', 'contacts', 'query', '--format', client,
                               '--query', query], env=env, text=True, capture_output=True)

    aerc = lookup('aerc')
    assert aerc.returncode == 0 and not aerc.stderr
    assert len(aerc.stdout.splitlines()) == 6
    assert all(len(row.split('\t')) == 2 for row in aerc.stdout.splitlines())
    neo = lookup('neomutt', 'Alex')
    assert neo.returncode == 0 and not neo.stderr
    assert neo.stdout.splitlines()[0] == 'TideContacts: 1 matching address(es)'
    assert neo.stdout.splitlines()[1].split('\t')[0] == 'person1@example.com'
    assert len(neo.stdout.splitlines()[1].split('\t')) == 3
    assert lookup('aerc', 'no-match').stdout == ''
    missing = lookup('neomutt', 'no-match')
    assert missing.returncode == 1 and len(missing.stdout.splitlines()) == 1
    assert json.loads(missing.stderr)['error']['code'] == 'no_matches'

    # NeoMutt quotes %s using shell quoting before executing the command.
    # Our final `--` also prevents query text from becoming a CLI option.
    prefix = shlex.quote(str(binary)) + ' --demo contacts query --format neomutt -- '
    marker = pathlib.Path(tmp) / 'must-not-exist'
    for query in ("O'Connor", '--database /tmp/another-book.db',
                  '$(touch ' + str(marker) + ')', '; touch ' + str(marker), 'a\nb'):
        result = subprocess.run(['sh', '-c', prefix + shlex.quote(query)], env=env,
                                text=True, capture_output=True)
        assert result.returncode in (0, 1), (query, result.stdout, result.stderr)
        lines = result.stdout.splitlines()
        assert lines and lines[0].startswith('TideContacts: '), (query, result.stdout)
        if result.returncode == 1:
            assert len(lines) == 1
        else:
            assert all(len(row.split('\t')) == 3 for row in lines[1:])
        assert not marker.exists()

    # The supplied aerc snippet does not interpolate query text into a shell.
    fragment = (project / 'contrib/mail-clients/aerc.conf').read_text()
    command = next(row.split('=', 1)[1].strip() for row in fragment.splitlines()
                   if row.startswith('address-book-cmd'))
    assert '%s' not in command
    command = command.replace('tidecontacts ', shlex.quote(str(binary)) + ' --demo ', 1)
    result = subprocess.run(['sh', '-c', command], env=env, text=True, capture_output=True)
    assert result.returncode == 0 and result.stdout == aerc.stdout
    assert not (pathlib.Path(tmp) / 'data/tidecontacts/tidecontacts.db').exists()
print('PASS: aerc/NeoMutt wire formats, no-match status, safe shell arguments and demo isolation')
