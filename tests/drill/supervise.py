"""Bound an untrusted command, including descendants that leave its process group.

Linux only: each supervisor is a subreaper, so even double-forked children
belong to it after their parent exits. No gotest code participates in cleanup.
"""
import ctypes
import json
import os
from pathlib import Path
import signal
import subprocess
import sys
import time


def descendants():
    parents, births = {}, {}
    for entry in Path('/proc').iterdir():
        if not entry.name.isdecimal():
            continue
        try:
            fields = (entry / 'stat').read_text().rsplit(')', 1)[1].split()
            parents[int(entry.name)] = int(fields[1])
            births[int(entry.name)] = fields[19]
        except (OSError, ValueError, IndexError):
            continue
    owned = {os.getpid()}
    while True:
        more = {pid for pid, parent in parents.items() if parent in owned}
        if more <= owned:
            return {pid: births[pid] for pid in owned - {os.getpid()}}
        owned |= more


def reap():
    while True:
        try:
            pid, _ = os.waitpid(-1, os.WNOHANG)
            if pid == 0:
                return
        except ChildProcessError:
            return


def clean():
    # Kill before reaping: a zombie's pid cannot be reused while it is ours.
    # Repeat for descendants adopted during the preceding pass.
    deadline = time.monotonic() + 5
    while True:
        for pid, birth in descendants().items():
            try:
                # A pidfd pins identity. Recheck the birth time after opening:
                # a descendant may have been reaped by its own parent between
                # the /proc scan and pidfd_open, and its number reused.
                fd = os.pidfd_open(pid)
                try:
                    current = Path(f'/proc/{pid}/stat').read_text().rsplit(')', 1)[1].split()[19]
                    if current == birth:
                        signal.pidfd_send_signal(fd, signal.SIGKILL)
                finally:
                    os.close(fd)
            except (ProcessLookupError, FileNotFoundError):
                pass
        reap()
        if not descendants():
            return
        if time.monotonic() >= deadline:
            raise RuntimeError('descendants survived cleanup')
        time.sleep(0.01)


def enable_subreaper():
    # Refuse unsupported kernels before starting anything we must later reap.
    fd = os.pidfd_open(os.getpid())
    os.close(fd)
    libc = ctypes.CDLL(None, use_errno=True)
    if libc.prctl(36, 1, 0, 0, 0) != 0:  # PR_SET_CHILD_SUBREAPER
        raise OSError(ctypes.get_errno(), 'cannot become a subreaper')


def supervise(seconds, status_path, command):
    enable_subreaper()
    stopped = []
    def stop(sig, _frame):
        stopped.append(sig)
    signal.signal(signal.SIGTERM, stop)
    signal.signal(signal.SIGINT, stop)
    began = time.monotonic()
    result = {'exit': None, 'timeout': False, 'interrupted': False}
    proc = None
    try:
        proc = subprocess.Popen(command, start_new_session=True)
        deadline = time.monotonic() + seconds
        while proc.poll() is None:
            if stopped or time.monotonic() >= deadline:
                result['interrupted'] = bool(stopped)
                result['timeout'] = not stopped
                break
            time.sleep(0.02)
        result['exit'] = proc.returncode
    finally:
        try:
            clean()
        except Exception as err:
            result['cleanup_error'] = str(err)
        # clean() reaps after killing, including the command itself.
        result['seconds'] = time.monotonic() - began
        Path(status_path).write_text(json.dumps(result) + '\n')
    return 0 if not any(result.get(k) for k in ('timeout', 'interrupted', 'cleanup_error')) else 1


if __name__ == '__main__':
    sys.exit(supervise(float(sys.argv[1]), sys.argv[2], sys.argv[3:]))
