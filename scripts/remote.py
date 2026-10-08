#!/usr/bin/env python3
"""Run a local shell script over SSH using ignored .env settings.

Usage: python3 scripts/remote.py [--sudo] path/to/script.sh
Requires an already trusted host key in ~/.ssh/known_hosts.
"""
import os
from pathlib import Path
import shlex
import subprocess
import sys

ROOT = Path(__file__).resolve().parent.parent


def settings():
    values = {}
    for raw in (ROOT / '.env').read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith('#'):
            continue
        key, separator, value = line.removeprefix('export ').partition('=')
        if not separator:
            continue
        value = value.strip()
        if len(value) >= 2 and value[0] == value[-1] and value[0] in "\"'":
            value = value[1:-1]
        values[key.strip()] = value
    return values


def main():
    cfg = settings()
    # OpenSSH invokes SSH_ASKPASS with a single password prompt argument.
    if os.environ.get('STT_SSH_ASKPASS') == '1' and len(sys.argv) == 2 and 'password' in sys.argv[1].lower():
        sys.stdout.write(cfg.get('DEPLOY_SSH_PASS', '') + '\n')
        return 0
    sudo = '--sudo' in sys.argv[1:]
    paths = [arg for arg in sys.argv[1:] if arg != '--sudo']
    if len(paths) != 1:
        sys.exit('Usage: remote.py [--sudo] script.sh')
    for name in ('DEPLOY_SSH_HOST', 'DEPLOY_SSH_USER'):
        if not cfg.get(name):
            sys.exit('Missing setting: ' + name)
    port = cfg.get('DEPLOY_SSH_PORT') or '22'
    if not port.isdecimal() or not 1 <= int(port) <= 65535:
        sys.exit('Invalid SSH port')
    script = Path(paths[0]).read_text()
    command = 'bash -s'
    payload = script
    if sudo:
        if not cfg.get('DEPLOY_SSH_PASS'):
            sys.exit('DEPLOY_SSH_PASS is required for sudo mode')
        command = "sudo -S -p '' -- bash -c " + shlex.quote(script)
        payload = cfg['DEPLOY_SSH_PASS'] + '\n'
    args = ['ssh', '-F', '/dev/null', '-o', 'StrictHostKeyChecking=yes',
            '-o', 'ConnectTimeout=10', '-o', 'NumberOfPasswordPrompts=1',
            '-o', 'ServerAliveInterval=15', '-o', 'ServerAliveCountMax=3',
            '-p', port, '-l', cfg['DEPLOY_SSH_USER']]
    if cfg.get('DEPLOY_SSH_KEY_PATH'):
        args += ['-i', str(Path(cfg['DEPLOY_SSH_KEY_PATH']).expanduser())]
    elif cfg.get('DEPLOY_SSH_PASS'):
        args += ['-o', 'PreferredAuthentications=password', '-o', 'PubkeyAuthentication=no']
    args += ['--', cfg['DEPLOY_SSH_HOST'], command]
    env = dict(os.environ, SSH_ASKPASS=str(Path(__file__).resolve()),
               SSH_ASKPASS_REQUIRE='force', STT_SSH_ASKPASS='1', DISPLAY=':0')
    proc = subprocess.run(args, input=payload, stdout=subprocess.PIPE,
                          stderr=subprocess.STDOUT, text=True, env=env,
                          start_new_session=True)
    output = proc.stdout
    for name in ('DEPLOY_SSH_PASS', 'DEPLOY_SSH_HOST', 'DEPLOY_SSH_USER'):
        if cfg.get(name):
            output = output.replace(cfg[name], '[redacted]')
    print(output, end='', flush=True)
    return proc.returncode


if __name__ == '__main__':
    try:
        sys.exit(main())
    except (OSError, ValueError):
        sys.exit('Unable to read SSH settings or execute SSH; check local configuration.')
