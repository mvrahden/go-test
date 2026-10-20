"""Tests of the pristine orchestrator; these never apply or run a mutant."""
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from unittest import mock

import driver


class PlanningTests(unittest.TestCase):
    def test_shards_cover_every_mutant_once_and_balance_long_jobs(self):
        names = ['a', 'b', 'c', 'd', 'e']
        modes = dict.fromkeys(names, 'shared')
        weights = dict(a=100, b=80, c=20, d=10, e=10)
        result = driver.schedule(names, modes, weights, 2)
        self.assertEqual(sorted(sum(result, [])), names)
        self.assertEqual([sum(weights[n] for n in shard) for shard in result], [110, 110])
        self.assertEqual(result, driver.schedule(list(reversed(names)), modes, weights, 2))

    def test_shards_with_different_cpu_counts_plan_alike(self):
        names = ['m%02d' % i for i in range(40)]
        modes = {n: 'alone' if i % 7 == 0 else 'shared' for i, n in enumerate(names)}
        weights = {n: 10 + (i * 37) % 90 for i, n in enumerate(names)}
        for environ, cpus in [({'DRILL_JOBS': '2'}, 8), ({'DRILL_JOBS': '4'}, 8), ({}, 4), ({}, 12)]:
            with self.subTest(environ=environ, cpus=cpus):
                two, _ = driver.plan(names, modes, weights, 2, {'DRILL_JOBS': '2'}, 4)
                other, _ = driver.plan(names, modes, weights, 2, environ, cpus)
                self.assertEqual(sorted(two[0] + other[1]), names)
                self.assertEqual(two, other)

    def test_isolated_time_is_not_divided_by_worker_count(self):
        self.assertEqual(driver.schedule(['a', 'b', 'c'], dict(a='alone', b='shared', c='shared'),
                                         dict(a=10, b=10, c=10), 2), [['a'], ['b', 'c']])

    def test_shards_that_disagree_fail_the_check(self):
        def selection(shard, assignment, mutants=None):
            return dict(shard=shard, shards=len(assignment), assignment=assignment,
                        mutants=dict.fromkeys(assignment[shard - 1] if mutants is None else mutants, {}))
        agreed = [['a', 'c'], ['b']]
        cases = {'agree': ([selection(1, agreed), selection(2, agreed)], []),
                 'different plans': ([selection(1, agreed), selection(2, [['a'], ['b', 'c']])],
                                     ['shards computed different assignments']),
                 'missing shard': ([selection(1, agreed)], ['expected one selection per shard 1..2, got [1]']),
                 'stray selection': ([selection(1, agreed, ['a']), selection(2, agreed)],
                                     ['shard 1 selected other mutants than its assignment names'])}
        for label, (selections, problems) in cases.items():
            with self.subTest(label), tempfile.TemporaryDirectory() as tmp:
                paths = []
                for i, s in enumerate(selections):
                    paths.append(Path(tmp) / f'{i}.json')
                    paths[-1].write_text(json.dumps(s))
                self.assertEqual(driver.check_shards(paths), problems)

    def test_incomplete_or_duplicate_plans_are_rejected(self):
        for text in ['mutant\ta\tshared\n', 'mutant\ta\tshared\nmutant\ta\tshared\n',
                     'bad\ta\tno such test\n', 'mutant\ta\tshared\nrun\tb\t-\t./x\n']:
            with self.subTest(text=text), self.assertRaises(ValueError):
                driver.read_plan(text, ['a'])

    def test_valid_plan_preserves_literal_filters(self):
        modes, runs = driver.read_plan('mutant\ta\talone\nrun\ta\t^(TestA)$/^(TestB)$\t./x ./y\n', ['a'])
        self.assertEqual(modes, {'a': 'alone'})
        self.assertEqual(runs['a'], [('^(TestA)$/^(TestB)$', ['./x', './y'])])

    def test_build_failure_cannot_be_accepted_as_an_assertion_failure(self):
        self.assertTrue(driver.build_failure('{"Action":"build-fail","ImportPath":"p"}\n'))
        self.assertFalse(driver.build_failure('{"Action":"fail","Test":"TestA"}\n'))
        with self.assertRaises(ValueError):
            driver.build_failure('not JSON')

    def test_the_judge_runs_without_the_hosts_ci_settings(self):
        host = dict(PATH='/bin', CI='true', GOTEST_CI='1', GOTEST_UPDATE_SNAPSHOTS='1',
                    GOTEST_CACHE_DIR='/shared', GITHUB_ACTIONS='true', GOFLAGS='-mod=mod')
        env = driver.worker_env(host, Path('/w'), Path('/w/tmp'), 3)
        for name in ('CI', 'GOTEST_CI', 'GOTEST_UPDATE_SNAPSHOTS', 'GITHUB_ACTIONS'):
            self.assertNotIn(name, env)
        self.assertEqual(env['GOTEST_CACHE_DIR'], '/w/cache/judge')
        self.assertEqual((env['PATH'], env['GOFLAGS'], env['GOMAXPROCS']), ('/bin', '-mod=mod -p=3', '3'))

    def test_missing_or_stale_result_and_dead_worker_fail_closed(self):
        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp) / 'result'
            clean = {'exit': 0, 'timeout': False}
            self.assertFalse(driver.read_result(path, 'a', clean, 0)['caught'])
            path.write_text(json.dumps({'name': 'b', 'caught': True}))
            self.assertFalse(driver.read_result(path, 'a', clean, 0)['caught'])
            path.write_text(json.dumps({'name': 'a', 'caught': True}))
            self.assertTrue(driver.read_result(path, 'a', clean, 0)['caught'])
            for status, code in [({}, 0), ({'exit': -9}, 0), ({'exit': 0, 'timeout': True}, 0),
                                 ({'exit': 0, 'cleanup_error': 'failed'}, 0), (clean, 1)]:
                self.assertFalse(driver.read_result(path, 'a', status, code)['caught'])


class WorkerTests(unittest.TestCase):
    # Exercise the orchestration with synthetic events, not a real CLI, patch
    # or Go invocation. Only grep actually runs, against the synthetic log.
    def worker(self, stream, exit_code=1, build_code=0, kept=None):
        with tempfile.TemporaryDirectory() as tmp:
            root = Path(tmp)
            snapshot = root / 'snapshot'
            snapshot.mkdir()
            patch = root / 'synthetic.patch'
            patch.write_text('Expect: a named failure\n--- a/unused\n')
            result = root / 'result.json'
            caches = [root / 'worker' / 'cache' / d / 'entry' for d in ('children', 'cli')]
            for entry in caches:
                entry.parent.mkdir(parents=True)
                entry.write_text('from an earlier mutant')
            spec = dict(name='synthetic', work=str(root / 'worker'), snapshot=str(snapshot),
                        patch=str(patch), log=str(root / 'output.log'), result=str(result),
                        runs=[('-', ['./synthetic'])], judge='synthetic-judge', cpus=1)
            real_run = subprocess.run
            def run(command, **kwargs):
                if command[0] == 'grep':
                    return real_run(command, **kwargs)
                if command[0] == 'go':
                    return subprocess.CompletedProcess(command, build_code)
                if '-json' in command:
                    kwargs['stdout'].write(stream)
                    return subprocess.CompletedProcess(command, exit_code)
                if 'summary' in command:
                    kwargs['stdout'].write('a named failure\n')
                return subprocess.CompletedProcess(command, 0)
            with mock.patch.object(driver.subprocess, 'run', side_effect=run):
                driver.worker(spec)
            if kept is not None:
                kept.extend(e.parent.name for e in caches if e.exists())
            return json.loads(result.read_text())

    def test_assertion_failure_matches_the_named_check(self):
        result = self.worker('{"Action":"fail","Test":"TestCheck"}\n')
        self.assertTrue(result['caught'])
        self.assertEqual(set(result['phases']), {'copy', 'patch', 'build', 'judge', 'render'})

    def test_only_the_go_validated_cache_outlives_a_mutant(self):
        kept = []
        self.worker('{"Action":"fail","Test":"TestCheck"}\n', kept=kept)
        self.assertEqual(kept, ['cli'])

    def test_build_failure_is_never_a_catch(self):
        result = self.worker('{"Action":"build-fail","ImportPath":"p"}\n', exit_code=2)
        self.assertFalse(result['caught'])
        self.assertIn('harness does not compile', result['reason'])
        self.assertFalse(self.worker('', build_code=1)['caught'])

    def test_a_survivor_cannot_pass_through_matching_log_text(self):
        self.assertFalse(self.worker('{"Action":"pass"}\n', exit_code=0)['caught'])

    def test_empty_malformed_and_abnormal_runs_fail(self):
        for stream, code in [('', 2), ('not JSON', 1), ('[]', 1), ('{"Action":"fail"}\n', -9)]:
            with self.subTest(stream=stream, code=code):
                self.assertFalse(self.worker(stream, code)['caught'])


@unittest.skipUnless(sys.platform == 'linux', 'Linux subreaper')
class SupervisorTests(unittest.TestCase):
    def supervise(self, source, seconds=3, interrupt=False):
        with tempfile.TemporaryDirectory() as tmp:
            status, pidfile = Path(tmp) / 'status', Path(tmp) / 'pid'
            proc = subprocess.Popen([sys.executable, str(driver.HERE / 'supervise.py'), str(seconds),
                                     str(status), sys.executable, '-c', source, str(pidfile)])
            try:
                limit = time.monotonic() + 3
                while not pidfile.exists() and time.monotonic() < limit:
                    time.sleep(0.01)
                self.assertTrue(pidfile.exists(), 'child never started')
                child = int(pidfile.read_text())
                if interrupt:
                    proc.send_signal(signal.SIGTERM)
                code = proc.wait(timeout=8)
                result = json.loads(status.read_text())
                with self.assertRaises(ProcessLookupError):
                    os.kill(child, 0)
                return code, result
            finally:
                if proc.poll() is None:
                    proc.kill()
                    proc.wait()

    def test_normal_exit_reaps_double_forked_detached_child(self):
        source = '''import os,sys,time
pid=os.fork()
if pid==0:
 os.setsid()
 if os.fork(): os._exit(0)
 open(sys.argv[1],'w').write(str(os.getpid()))
 time.sleep(60)
else:
 os.waitpid(pid,0)
 # Wait until the detached child published its pid before exiting.
 while not os.path.exists(sys.argv[1]): time.sleep(.01)
'''
        code, result = self.supervise(source)
        self.assertEqual(code, 0)
        self.assertEqual(result['exit'], 0)
        self.assertFalse(result['timeout'])

    def test_timeout_cleans_a_child_in_a_separate_session(self):
        source = '''import subprocess,sys,time
p=subprocess.Popen([sys.executable,'-c','import time;time.sleep(60)'],start_new_session=True)
open(sys.argv[1],'w').write(str(p.pid))
time.sleep(60)
'''
        code, result = self.supervise(source, seconds=.3)
        self.assertNotEqual(code, 0)
        self.assertTrue(result['timeout'])

    def test_cancellation_cleans_descendants(self):
        source = '''import subprocess,sys,time
p=subprocess.Popen([sys.executable,'-c','import time;time.sleep(60)'],start_new_session=True)
open(sys.argv[1],'w').write(str(p.pid))
time.sleep(60)
'''
        code, result = self.supervise(source, interrupt=True)
        self.assertNotEqual(code, 0)
        self.assertTrue(result['interrupted'])


if __name__ == '__main__':
    unittest.main()
