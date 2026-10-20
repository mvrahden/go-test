"""Mutation drill orchestration; the shell entry point remains drill.sh.

Only --worker applies a mutation to a copied source tree. Planning, scheduling, supervision
and verdict accounting run outside it, against pristine code.
"""
import concurrent.futures
import contextlib
import json
import math
import os
from pathlib import Path
import queue
import re
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time

import supervise as process_supervisor

HERE = Path(__file__).resolve().parent
ROOT = HERE.parent.parent
ANSI = re.compile(r'\x1b\[[0-9;]*m')


# Each shared mutant's share of a shard's wall time. Fixed, so shards whose
# runners differ in CPUs still compute one plan and split it exactly.
SHARED_DIVISOR = 2


def schedule(names, modes, weights, shards):
    """Longest first, greedy projected wall time; deterministic across shards."""
    buckets = [[] for _ in range(shards)]
    load = [0.0] * shards
    for name in sorted(names, key=lambda n: (-weights.get(n, 30), n)):
        slot = min(range(shards), key=lambda i: (load[i], i))
        buckets[slot].append(name)
        load[slot] += weights.get(name, 30) / (SHARED_DIVISOR if modes[name] == 'shared' else 1)
    return buckets


def plan(names, modes, weights, shards, environ, cpus):
    """Every shard's mutants, and this shard's concurrent worker count.

    The assignment reads only what every shard shares; the local CPU count
    and DRILL_JOBS set the workers alone.
    """
    return schedule(names, modes, weights, shards), worker_count(environ, cpus)


def worker_count(environ, cpus):
    return positive(environ.get('DRILL_JOBS', max(1, cpus // 2)), 'DRILL_JOBS')


def check_shards(paths):
    """Problems with the selection files of one sharded run, if any."""
    try:
        selections = [json.loads(Path(p).read_text()) for p in paths]
        for s in selections:
            s['shard'], s['shards'], s['assignment'], s['mutants']
    except (OSError, ValueError, TypeError, KeyError):
        return ['unreadable selection file']
    if not selections:
        return ['no selection files']
    problems = []
    first = selections[0]
    if any(s['assignment'] != first['assignment'] or s['shards'] != first['shards'] for s in selections):
        problems.append('shards computed different assignments')
    shards = sorted(s['shard'] for s in selections)
    if shards != list(range(1, first['shards'] + 1)):
        problems.append('expected one selection per shard 1..%d, got %s' % (first['shards'], shards))
    for s in selections:
        if not 1 <= s['shard'] <= len(s['assignment']) or sorted(s['mutants']) != sorted(s['assignment'][s['shard'] - 1]):
            problems.append('shard %d selected other mutants than its assignment names' % s['shard'])
    return problems


def read_plan(text, names):
    modes, runs = {}, {n: [] for n in names}
    for line in text.splitlines():
        fields = line.split('\t')
        if len(fields) == 3 and fields[0] == 'mutant' and fields[1] in runs:
            _, name, mode = fields
            if name in modes or mode not in ('alone', 'shared'):
                raise ValueError('duplicate or invalid mutant: ' + name)
            modes[name] = mode
        elif len(fields) == 4 and fields[0] == 'run' and fields[1] in runs:
            runs[fields[1]].append((fields[2], fields[3].split()))
        else:
            raise ValueError('invalid judge plan: ' + line)
    if set(modes) != set(names) or any(not r for r in runs.values()):
        raise ValueError('incomplete judge plan')
    return modes, runs


def build_failure(stream):
    for line in stream.splitlines():
        try:
            event = json.loads(line)
        except ValueError:
            raise ValueError('judge emitted a non-JSON line') from None
        if not isinstance(event, dict) or not isinstance(event.get('Action'), str):
            raise ValueError('judge emitted an invalid event')
        if event.get('Action') == 'build-fail':
            return True
    return False


def read_result(path, name, supervisor, supervisor_exit):
    if not isinstance(supervisor, dict):
        supervisor = {}
    if supervisor_exit != 0 or supervisor.get('exit') != 0 or any(
            supervisor.get(k) for k in ('timeout', 'interrupted', 'cleanup_error')):
        reason = 'worker or supervisor exited without a valid result'
        if supervisor.get('timeout'):
            reason = 'independent watchdog deadline exceeded'
        elif supervisor.get('interrupted'):
            reason = 'worker cancelled'
        elif supervisor.get('cleanup_error'):
            reason = 'process cleanup failed: ' + supervisor['cleanup_error']
        return {'name': name, 'caught': False, 'reason': reason,
                'seconds': supervisor.get('seconds', 0), 'supervisor': supervisor}
    try:
        result = json.loads(Path(path).read_text())
        if not isinstance(result, dict) or result.get('name') != name or not isinstance(result.get('caught'), bool):
            raise ValueError('invalid worker result')
        return result
    except (OSError, ValueError):
        return {'name': name, 'caught': False, 'reason': 'worker produced no valid terminal result'}


# Variables that change what the checks under judgment do: CI=true makes the
# CLI set GOTEST_CI, under which a missing snapshot baseline fails on its own.
HOST_ONLY = ('CI', 'GITHUB_ACTIONS', 'GITHUB_STEP_SUMMARY')


def worker_env(base, work, tmp, cpus):
    """The judge's environment, the same on a CI runner as on a laptop."""
    env = {k: v for k, v in base.items() if k not in HOST_ONLY and not k.startswith('GOTEST_')}
    env.update(TMPDIR=str(tmp), GOTEST_CACHE_DIR=str(work / 'cache' / 'judge'),
               GOTEST_TEST_CACHE_DIR=str(work / 'cache' / 'children'),
               GOTEST_TEST_CLI_CACHE_DIR=str(work / 'cache' / 'cli'),
               GOMAXPROCS=str(cpus))
    env['GOFLAGS'] = env.get('GOFLAGS', '') + ' -p=' + str(cpus)
    return env


def worker(spec):
    started = time.monotonic()
    timings = {}
    name = spec['name']
    work = Path(spec['work'])
    tree, tmp = work / 'tree', work / 'tmp'
    log_path = Path(spec['log'])
    result = {'name': name, 'caught': False}
    env = worker_env(os.environ, work, tmp, spec['cpus'])
    def note_phase(label):
        Path(spec['log'] + '.phase.json').write_text(json.dumps(dict(phase=label, time=time.time())) + '\n')
    def phase(label, command, stdout=None):
        note_phase(label)
        began = time.monotonic()
        completed = subprocess.run(command, cwd=tree, env=env, stdin=subprocess.DEVNULL,
                                   stdout=stdout or log, stderr=log, check=False)
        timings[label] = timings.get(label, 0) + time.monotonic() - began
        return completed.returncode
    try:
        note_phase('copy')
        began = time.monotonic()
        # Go validates cache/cli and GOCACHE; gotest's own entries in
        # cache/children were written by an earlier mutant's CLI.
        for directory in (tree, tmp, work / 'cache' / 'children'):
            shutil.rmtree(directory, ignore_errors=True)
        shutil.copytree(spec['snapshot'], tree)
        tmp.mkdir()
        timings['copy'] = time.monotonic() - began
        with log_path.open('w') as log:
            if phase('patch', ['patch', '-p1', '-s', '--fuzz=0', '--forward',
                               '--no-backup-if-mismatch', '-r', '-', '-i', spec['patch']]):
                raise RuntimeError('patch does not apply, refresh it')
            # Keep the whole-source build contract; compile generated tests only
            # in the actual judge pipeline, whose build-fail events are checked.
            if phase('build', ['go', 'build', './...']):
                raise RuntimeError('mutant does not compile, refresh it')
            failed = False
            for i, (pattern, packages) in enumerate(spec['runs']):
                stream = Path(spec['log'] + f'.{i}.jsonl')
                args = [spec['judge'], '-json', '--parallel=' + str(spec['cpus']),
                        '--compile-parallel=' + str(spec['cpus'])]
                if pattern != '-':
                    args += ['-run', pattern]
                with stream.open('w') as events:
                    code = phase('judge', args + packages, events)
                if code not in (0, 1, 2):
                    raise RuntimeError(f'judge terminated abnormally ({code})')
                if build_failure(stream.read_text()):
                    raise RuntimeError('generated judge harness does not compile, refresh it')
                if not stream.stat().st_size:
                    raise RuntimeError('judge emitted no events')
                failed |= code != 0
                if phase('render', [spec['judge'], 'summary', '--input=' + str(stream),
                                    '--render-only', '--no-color']):
                    raise RuntimeError('could not render judge events')
            if not failed:
                raise RuntimeError('SURVIVED, the guards did not catch it')
        note_phase('expectations')
        # grep retains the patches' POSIX ERE meaning, not Python's regex dialect.
        plain = Path(spec['log'] + '.plain')
        plain.write_text(ANSI.sub('', log_path.read_text()))
        header = Path(spec['patch']).read_text().split('--- ', 1)[0]
        expects = [line[8:] for line in header.splitlines() if line.startswith('Expect: ')]
        if not expects:
            raise RuntimeError('no Expect: line')
        for expression in expects:
            if subprocess.run(['grep', '-qE', '--', expression, str(plain)], check=False).returncode:
                raise RuntimeError('caught by the wrong check; no line matches: ' + expression)
        result.update(caught=True, reason='caught')
    except Exception as err:
        result['reason'] = str(err)
    finally:
        result.update(seconds=time.monotonic() - started, phases=timings)
        Path(spec['result']).write_text(json.dumps(result) + '\n')
        note_phase('done')


def positive(value, label):
    n = int(value)
    if n < 1:
        raise ValueError(label + ' must be a positive integer')
    return n


def available_cpus():
    cpus = len(os.sched_getaffinity(0))
    # Containers can expose a large affinity mask but a much smaller quota.
    try:
        quota, period = Path('/sys/fs/cgroup/cpu.max').read_text().split()
        if quota != 'max':
            cpus = min(cpus, max(1, int(quota) // int(period)))
    except (OSError, ValueError, ZeroDivisionError):
        pass
    return cpus


def main(names):
    if sys.platform != 'linux':
        raise ValueError('the drill requires Linux for subreaper process cleanup')
    process_supervisor.enable_subreaper()
    cpus = available_cpus()
    jobs = worker_count(os.environ, cpus)
    shard_i, shard_n = [positive(v, 'DRILL_SHARD') for v in os.environ.get('DRILL_SHARD', '1/1').split('/')]
    if shard_i > shard_n:
        raise ValueError('DRILL_SHARD must be i/n with 1 <= i <= n')
    seconds = positive(os.environ.get('DRILL_TIMEOUT', '600'), 'DRILL_TIMEOUT')
    names = names or sorted(p.stem for p in (HERE / 'mutants').glob('*.patch'))
    if len(names) != len(set(names)):
        raise ValueError('a mutant was selected more than once')
    for name in names:
        if not re.fullmatch('[a-z0-9-]+', name) or not (HERE / 'mutants' / (name + '.patch')).is_file():
            raise ValueError('no mutant named ' + name)
    weights = {}
    if os.environ.get('DRILL_TIMINGS'):
        weights = json.loads(Path(os.environ['DRILL_TIMINGS']).read_text())
        if not isinstance(weights, dict) or any(not isinstance(v, (int, float)) or
                not math.isfinite(v) or v <= 0 for v in weights.values()):
            raise ValueError('DRILL_TIMINGS must map mutant names to positive finite seconds')
    started = time.monotonic()
    log_root = os.environ.get('DRILL_LOGS')
    if log_root:
        Path(log_root).mkdir(parents=True, exist_ok=True)
    # Concurrent invocations, including shards on one machine, never share
    # status files or mistake another invocation's verdict for their own.
    logs = Path(tempfile.mkdtemp(prefix=f'drill-{shard_i}-of-{shard_n}-', dir=log_root)).resolve()
    print('drill  logs: ' + str(logs), flush=True)
    active = set()
    lock = threading.RLock()
    stopping = threading.Event()
    def stop(_sig, _frame):
        stopping.set()
        with lock:
            for proc in active:
                proc.terminate()
    previous = {sig: signal.signal(sig, stop) for sig in (signal.SIGINT, signal.SIGTERM)}
    results = {}
    try:
        with tempfile.TemporaryDirectory(prefix='gotest-drill-') as scratch_dir, contextlib.ExitStack() as cleanup:
            # If a supervisor itself dies, its detached descendants are adopted
            # here and reaped before the source/cache directories are removed.
            cleanup.callback(process_supervisor.clean)
            scratch = Path(scratch_dir)
            snapshot = scratch / 'snapshot'
            ignored = shutil.ignore_patterns('.git', '.worktrees', 'node_modules', '__pycache__')
            def snapshot_ignore(directory, entries):
                skip = set(ignored(directory, entries))
                for entry in entries:
                    candidate = Path(directory, entry).resolve()
                    if candidate == logs or (log_root and candidate == Path(log_root).resolve()):
                        skip.add(entry)
                return skip
            shutil.copytree(ROOT, snapshot, ignore=snapshot_ignore)
            judge, clauses = scratch / 'gotest', scratch / 'clauses'
            trusted = snapshot / 'tests' / 'drill'
            def supervised(command, label, stdout, stderr, cwd=None):
                status = logs / (label + '.supervisor.json')
                status.unlink(missing_ok=True)
                with lock:
                    if stopping.is_set():
                        raise InterruptedError('drill cancelled')
                    proc = subprocess.Popen([sys.executable, str(trusted / 'supervise.py'),
                                             str(seconds), str(status)] + command,
                                            cwd=cwd, stdout=stdout, stderr=stderr)
                    active.add(proc)
                    if stopping.is_set():
                        proc.terminate()
                try:
                    code = proc.wait()
                finally:
                    with lock:
                        active.discard(proc)
                try:
                    observed = json.loads(status.read_text())
                except (OSError, ValueError):
                    observed = {}
                return code, observed
            # Bootstrap and planning use the same independent deadline and
            # cancellation cleanup as mutants; partial plans never schedule.
            with (logs / 'bootstrap.log').open('w') as output:
                for binary, package in [(judge, './cmd/gotest'), (clauses, './internal/speccatalog/clauses')]:
                    code, observed = supervised(['go', 'build', '-o', str(binary), package],
                                                binary.name + '-build', output, output, snapshot)
                    if code or observed.get('exit') != 0:
                        raise RuntimeError('could not build ' + binary.name + '; see bootstrap.log')
                # Go writes a tool into the shared build cache in place; a run
                # that resolves it meanwhile fails to exec it (text file busy).
                code, observed = supervised(['go', 'tool', '-n', 'test2json'], 'test2json-build',
                                            output, output, snapshot)
                if code or observed.get('exit') != 0:
                    raise RuntimeError('could not build test2json; see bootstrap.log')
                plan_path = logs / 'plan.tsv'
                with plan_path.open('w') as plan_out:
                    code, observed = supervised([str(clauses), '--judge-runs'] +
                        [str(snapshot / 'tests' / 'drill' / 'mutants' / (n + '.patch')) for n in names],
                        'plan', plan_out, output, snapshot)
                if code or observed.get('exit') != 0:
                    raise RuntimeError('could not read judge plan; see plan.tsv and bootstrap.log')
            modes, runs = read_plan(plan_path.read_text(), names)
            assignment, _ = plan(names, modes, weights, shard_n, os.environ, cpus)
            names = assignment[shard_i - 1]
            # The whole assignment, so --check-shards can prove the shards agree.
            (logs / f'selection-{shard_i}-of-{shard_n}.json').write_text(json.dumps(dict(
                shard=shard_i, shards=shard_n, assignment=assignment,
                mutants={n: dict(mode=modes[n], runs=runs[n], estimated_seconds=weights.get(n, 30)) for n in names}),
                indent=2) + '\n')
            def execute(worker_id, name, budget):
                work = scratch / ('w' + str(worker_id))
                work.mkdir(exist_ok=True)
                result = logs / (name + '.result.json')
                # A previous run's result must never count for this one.
                result.unlink(missing_ok=True)
                spec = dict(name=name, work=str(work), snapshot=str(snapshot),
                            patch=str(snapshot / 'tests' / 'drill' / 'mutants' / (name + '.patch')),
                            log=str(logs / (name + '.log')), result=str(result),
                            runs=runs[name], judge=str(judge), cpus=budget)
                config = work / 'job.json'
                config.write_text(json.dumps(spec))
                with (logs / (name + '.worker.log')).open('w') as output:
                    code, observed = supervised([sys.executable, str(trusted / 'driver.py'),
                                                 '--worker', str(config)], name, output, output)
                verdict = read_result(result, name, observed, code)
                if not observed or observed.get('cleanup_error'):
                    # Never reuse a worker, or judge a timing-sensitive mutant,
                    # when a failed supervisor may have left processes running.
                    stop(signal.SIGTERM, None)
                with lock:
                    result.write_text(json.dumps(verdict) + "\n")
                    results[name] = verdict
                    print(('drill  ' if verdict['caught'] else 'DRILL  ') + name + ': ' +
                          verdict['reason'] + f" ({verdict.get('seconds', 0):.1f}s)", flush=True)
            def phase(selected, parallel, budget):
                pending = queue.Queue()
                for name in selected:
                    pending.put(name)
                def consume(worker_id):
                    while not stopping.is_set():
                        try:
                            name = pending.get_nowait()
                        except queue.Empty:
                            return
                        execute(worker_id, name, budget)
                with concurrent.futures.ThreadPoolExecutor(max_workers=parallel) as pool:
                    futures = [pool.submit(consume, i) for i in range(parallel)]
                    try:
                        for future in futures:
                            future.result()  # worker exceptions fail the drill
                    except BaseException:
                        stop(signal.SIGTERM, None)
                        raise
            phase([n for n in names if modes[n] == 'shared'], jobs, max(1, cpus // jobs))
            phase([n for n in names if modes[n] == 'alone'], 1, cpus)
    finally:
        for sig, handler in previous.items():
            signal.signal(sig, handler)
        timings = {n: r['seconds'] for n, r in results.items() if r.get('caught') and r.get('seconds', 0) > 0}
        (logs / f'timings-{shard_i}-of-{shard_n}.json').write_text(json.dumps(timings, indent=2) + '\n')
    missing = set(names) - set(results)
    for name in sorted(missing):
        print('DRILL  ' + name + ': no terminal result', flush=True)
    print(f'drill  {len(results)}/{len(names)} mutants judged in {time.monotonic() - started:.1f}s', flush=True)
    return int(stopping.is_set() or bool(missing) or any(not r['caught'] for r in results.values()))


if __name__ == '__main__':
    try:
        if len(sys.argv) == 3 and sys.argv[1] == '--worker':
            worker(json.loads(Path(sys.argv[2]).read_text()))
            sys.exit(0)
        if len(sys.argv) == 3 and sys.argv[1] == '--check-shards':
            found = sorted(Path(sys.argv[2]).rglob('selection-*-of-*.json'))
            problems = check_shards(found)
            for problem in problems:
                print('DRILL  ' + problem, flush=True)
            if not problems:
                print(f'drill  {len(found)} shards agree on one assignment', flush=True)
            sys.exit(int(bool(problems)))
        sys.exit(main(sys.argv[1:]))
    except (ValueError, OSError, RuntimeError, subprocess.SubprocessError) as error:
        print('DRILL  ' + str(error), file=sys.stderr)
        sys.exit(1)
