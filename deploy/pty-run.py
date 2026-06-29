#!/usr/bin/env python3
# Drive `canopy start` through a PTY so it accepts the interactive key password
# non-interactively. Sends the password on prompt(s), streams output to a log,
# and (optionally) terminates after RUN_SECONDS. With RUN_SECONDS<=0 it runs
# until killed (used to keep the node up in the background).
#
#   python3 pty-run.py <run_seconds> <logfile>
#     run_seconds <= 0  -> run until killed (background node)
#     run_seconds  > 0  -> run for N seconds then terminate (one-time init)
#   env CANOPY_PW overrides the password (default: testpassword123)
#
# The canopy binary is expected at ~/veritas/node/canopy and is launched with
# the *current working directory* as its root, so run this from a directory that
# contains plugin/go/{pluginctl.sh, go-plugin} when the plugin is enabled.
import os, pty, select, sys, time

CANOPY = os.path.expanduser("~/veritas/node/canopy")
PASSWORD = (os.environ.get("CANOPY_PW", "testpassword123") + "\n").encode()
RUN_SECONDS = float(sys.argv[1]) if len(sys.argv) > 1 else 40.0
LOG = os.path.expanduser(sys.argv[2]) if len(sys.argv) > 2 else os.path.expanduser("~/veritas/node.log")

pid, fd = pty.fork()
if pid == 0:
    os.execv(CANOPY, [CANOPY, "start"])
else:
    start = time.time()
    sent = 0
    buf = b""
    out = open(LOG, "wb")
    while True:
        if RUN_SECONDS > 0 and time.time() - start > RUN_SECONDS:
            break
        r, _, _ = select.select([fd], [], [], 1.0)
        if fd in r:
            try:
                data = os.read(fd, 4096)
            except OSError:
                break
            if not data:
                break
            out.write(data); out.flush()
            try:
                sys.stdout.buffer.write(data); sys.stdout.flush()
            except Exception:
                pass
            buf += data
            low = buf.lower()
            if sent < 3 and (b"password" in low or b"confirm" in low or b"re-enter" in low):
                os.write(fd, PASSWORD)
                sent += 1
                buf = b""
                time.sleep(0.4)
    if RUN_SECONDS > 0:
        for sig in (15, 9):
            try: os.kill(pid, sig)
            except Exception: pass
            time.sleep(1)
    out.close()
