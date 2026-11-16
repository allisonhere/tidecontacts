#!/usr/bin/env python3
"""Install the built local contacts bridge; never opens either app's database."""
import os
import pathlib
import shutil
import tempfile

project = pathlib.Path(__file__).resolve().parent.parent
config = pathlib.Path(os.environ.get('XDG_CONFIG_HOME') or pathlib.Path.home() / '.config')
if not config.is_absolute():
    raise SystemExit('XDG_CONFIG_HOME must be absolute')
destination = config / 'tidemail' / 'plugins' / 'tidecontacts'
destination.mkdir(parents=True, exist_ok=True)
for name in ('tidecontacts-plugin', 'tidecontacts', 'plugin.toml'):
    fd, temporary = tempfile.mkstemp(prefix='.' + name + '-', dir=destination)
    os.close(fd)
    shutil.copy2(project / 'bin' / 'plugins' / 'tidecontacts' / name, temporary)
    os.replace(temporary, destination / name)
print('Installed TideContacts plugin:', destination)
print('Start the updated TideMail build and press C. No contacts were read or changed.')
