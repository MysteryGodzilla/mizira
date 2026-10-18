# Copyright (C) 2026 MysteryGodzilla
# Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
# SPDX-License-Identifier: GPL-3.0-only
"""Hourly and daily caps for tools that cost money, per person and overall.

    from metald_tools import usagecap
    over = usagecap.take("imagegen", nick, per_hour=5, per_day=20, day_total=60)
    if over: print(over)          # a message for the model; nothing was counted
    ...
    usagecap.give_back("imagegen", nick)   # the job failed before it cost anything

Uses are kept for 24 hours in usage-<tool>.json under METALD_DATADIR. A cap of 0 is no cap.
"""

import json
import os
import tempfile
import time

HOUR, DAY = 3600, 86400
LOCK_WAIT = 10


def _path(tool: str) -> str:
    return os.path.join(os.environ.get("METALD_DATADIR") or tempfile.gettempdir(), f"usage-{tool}.json")


class _Locked:
    """An exclusive lock file beside the state; one left by a crashed run is taken over after LOCK_WAIT."""

    def __init__(self, path):
        self.lock = path + ".lock"

    def __enter__(self):
        deadline = time.time() + LOCK_WAIT
        while True:
            try:
                os.close(os.open(self.lock, os.O_CREAT | os.O_EXCL | os.O_WRONLY, 0o600))
                return self
            except FileExistsError:
                if time.time() > deadline:
                    os.remove(self.lock)
                    deadline = time.time() + LOCK_WAIT
                time.sleep(0.05)

    def __exit__(self, *exc):
        try:
            os.remove(self.lock)
        except OSError:
            pass


def _load(path: str, now: float) -> list:
    try:
        with open(path, encoding="utf-8") as fh:
            uses = json.load(fh).get("uses", [])
    except (OSError, ValueError, AttributeError):
        uses = []
    return [u for u in uses if isinstance(u, list) and len(u) == 2 and now - u[0] < DAY]


def _save(path: str, uses: list) -> None:
    tmp = path + ".tmp"
    with open(tmp, "w", encoding="utf-8") as fh:
        json.dump({"uses": uses}, fh)
    os.replace(tmp, path)


def _wait(times: list, window: int, cap: int, now: float) -> str:
    """How long until one of the counted uses leaves the window, in words."""
    oldest = sorted(times)[len(times) - cap]
    minutes = max(1, round((oldest + window - now) / 60))
    return f"{minutes} minute{'s' if minutes != 1 else ''}" if minutes < 120 else f"{round(minutes / 60)} hours"


def take(tool: str, who: str, per_hour: int = 0, per_day: int = 0, day_total: int = 0,
         now: float = None) -> str:
    """Count one use by who, or return why not (and count nothing)."""
    now = time.time() if now is None else now
    who = (who or "unknown").lower()
    path = _path(tool)
    with _Locked(path):
        uses = _load(path, now)
        mine = [t for t, w in uses if w == who]
        hour = [t for t in mine if now - t < HOUR]
        if per_hour and len(hour) >= per_hour:
            return (f"Limit reached: {who} has had {len(hour)} in the last hour (the limit is {per_hour} an hour); "
                    f"the next is possible in {_wait(hour, HOUR, per_hour, now)}. Say so; don't retry.")
        if per_day and len(mine) >= per_day:
            return (f"Limit reached: {who} has had {len(mine)} in the last 24 hours (the limit is {per_day} a day); "
                    f"the next is possible in {_wait(mine, DAY, per_day, now)}. Say so; don't retry.")
        if day_total and len(uses) >= day_total:
            return (f"Limit reached: everyone together has had {len(uses)} in the last 24 hours (the limit is "
                    f"{day_total}); try again in {_wait([t for t, _ in uses], DAY, day_total, now)}. Say so; don't retry.")
        _save(path, uses + [[now, who]])
    return ""


def give_back(tool: str, who: str) -> None:
    """Uncount who's latest use."""
    who = (who or "unknown").lower()
    path = _path(tool)
    with _Locked(path):
        uses = _load(path, time.time())
        for i in range(len(uses) - 1, -1, -1):
            if uses[i][1] == who:
                del uses[i]
                break
        _save(path, uses)
