#!/usr/bin/env python3
"""Exercise the real scripting executable without opening a terminal or live data."""
import json
import os
import pathlib
import subprocess
import tempfile

binary = pathlib.Path(__file__).resolve().parent.parent / 'bin/tidecontacts'
with tempfile.TemporaryDirectory(prefix='tidecontacts-cli-') as root:
    env = dict(os.environ, TERM='dumb', NO_COLOR='1',
               XDG_CONFIG_HOME=root + '/config', XDG_DATA_HOME=root + '/data',
               XDG_STATE_HOME=root + '/state')

    def command(*args, value=None, code=0):
        result = subprocess.run([str(binary), '--demo', *args],
                                input=json.dumps(value) if value is not None else '',
                                env=env, text=True, capture_output=True, check=False)
        assert result.returncode == code, (args, result.returncode, result.stderr)
        assert '\x1b' not in result.stdout + result.stderr
        if code:
            assert not result.stdout
            return json.loads(result.stderr)
        assert not result.stderr, result.stderr
        return json.loads(result.stdout)

    assert len(command('contacts', 'list')['data']) == 6
    person = command('contacts', 'create', '--input', '-', value={
        'DisplayName': 'Script Person', 'Methods': [{'Kind': 'email', 'Value': 'script@example.com'}]
    })['data']
    task = command('tasks', 'create', '--input', '-', value={
        'Title': 'Script task', 'Entities': [{'ID': person['ID']}]
    })['data']
    command('tasks', 'update', task['ID'], '--input', '-', value={
        'Revision': task['Revision'], 'Priority': 3
    })
    stale = command('tasks', 'update', task['ID'], '--input', '-', value={
        'Revision': task['Revision'], 'Title': 'Stale edit'
    }, code=5)
    assert stale['error']['code'] == 'conflict'
    command('tasks', 'complete', task['ID'])
    pipeline = command('pipelines', 'create', '--input', '-', value={
        'Name': 'Script pipeline', 'Stages': [{'Name': 'New'}, {'Name': 'Done'}]
    })
    item = command('items', 'create', '--input', '-', value={
        'Title': 'Script opportunity', 'PipelineID': pipeline['data']['ID'],
        'StageID': pipeline['data']['Stages'][0]['ID']
    })['data']
    command('items', 'move', item['ID'], '--stage', pipeline['data']['Stages'][1]['ID'])
    command('pipelines', 'update', pipeline['data']['ID'], '--input', '-', '--if-match', pipeline['etag'],
            value={'Name': 'Renamed script pipeline'})
    command('contacts', 'delete', person['ID'], code=2)
    command('contacts', 'delete', person['ID'], '--yes')
    command('contacts', 'restore', person['ID'], '--yes')
    archive = command('crm', 'export')['data']
    assert archive['format'] == 'tidecontacts-crm'
    command('crm', 'import', '--input', '-', value=archive)
    assert 'tasks' in command('schema', 'tasks')['data']['resources']
    assert not (pathlib.Path(root) / 'data/tidecontacts/tidecontacts.db').exists()
print('PASS: JSON CLI CRUD, links, revisions, pipeline tokens, archive, errors and demo isolation')
