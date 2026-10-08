#!/usr/bin/env python3
"""Run one CPU transcription, recording wall time, RSS and temperature.

Run on the Raspberry from ~/stt-benchmark. Results are private local artifacts.
"""
import argparse
import json
from pathlib import Path
import subprocess
import time


def temperature():
    path = Path('/sys/class/thermal/thermal_zone0/temp')
    return int(path.read_text()) / 1000 if path.exists() else None


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('--model', required=True)
    parser.add_argument('--audio', required=True)
    parser.add_argument('--language', choices=['en', 'ru'], required=True)
    parser.add_argument('--name', required=True)
    parser.add_argument('--threads', type=int, default=4)
    parser.add_argument('--timeout', type=int, default=1800)
    args = parser.parse_args()
    if Path(args.name).name != args.name or args.name in ('.', '..'):
        parser.error('name must be a single filename component')
    result = Path('results') / args.name
    result.mkdir(parents=True, exist_ok=False)
    duration = float(subprocess.check_output([
        'ffprobe', '-v', 'error', '-show_entries', 'format=duration',
        '-of', 'default=noprint_wrappers=1:nokey=1', args.audio], text=True))
    command = ['whisper.cpp/build/bin/whisper-cli', '-m', args.model,
               '-f', args.audio, '-l', args.language, '-t', str(args.threads),
               '-ng', '-otxt', '-osrt', '-oj', '-of', str(result / 'transcript')]
    wrapper = ['/usr/bin/time', '-f', '%e %M', '-o', str(result / 'resources.txt'),
               'timeout', '--signal=TERM', '--kill-after=10', str(args.timeout)]
    start = time.monotonic()
    temperatures = []
    with (result / 'stdout.log').open('w') as stdout, (result / 'stderr.log').open('w') as stderr:
        process = subprocess.Popen(wrapper + command, stdout=stdout, stderr=stderr)
        while True:
            current = temperature()
            if current is not None:
                temperatures.append(current)
            try:
                code = process.wait(timeout=5)
                break
            except subprocess.TimeoutExpired:
                pass
    elapsed = time.monotonic() - start
    resources = (result / 'resources.txt').read_text().strip().splitlines()[-1].split()
    metrics = {
        'name': args.name, 'model': args.model, 'audio': args.audio,
        'language': args.language, 'threads': args.threads,
        'duration_seconds': duration, 'wall_seconds': elapsed,
        'rtf': elapsed / duration, 'peak_rss_mib': int(resources[1]) / 1024,
        'temperature_start_c': temperatures[0] if temperatures else None,
        'temperature_max_c': max(temperatures) if temperatures else None,
        'exit_code': code, 'command': command,
    }
    (result / 'metrics.json').write_text(json.dumps(metrics, indent=2) + '\n')
    print(json.dumps(metrics), flush=True)
    return code


if __name__ == '__main__':
    raise SystemExit(main())
