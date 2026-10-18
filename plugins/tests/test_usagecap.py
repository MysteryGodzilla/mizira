#!/usr/bin/env python3
# Copyright (C) 2026 MysteryGodzilla
# Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
# SPDX-License-Identifier: GPL-3.0-only

import os
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

sys.path[:0] = [os.path.join(os.path.dirname(__file__), "..", "lib")]

from metald_tools import usagecap

T0 = time.time()


class UsageCapTests(unittest.TestCase):
    def setUp(self):
        d = tempfile.TemporaryDirectory()
        self.addCleanup(d.cleanup)
        p = patch.dict(os.environ, {"METALD_DATADIR": d.name})
        p.start()
        self.addCleanup(p.stop)

    def take(self, who, at, **caps):
        return usagecap.take("t", who, now=T0 + at, **caps)

    def test_hourly_cap_per_person(self):
        for i in range(2):
            self.assertEqual(self.take("alice", i * 60, per_hour=2), "")
        over = self.take("Alice", 300, per_hour=2)
        self.assertIn("limit is 2 an hour", over)
        self.assertIn("55 minutes", over)
        self.assertEqual(self.take("bob", 300, per_hour=2), "", "another person has their own count")
        self.assertEqual(self.take("alice", 3601, per_hour=2), "", "the oldest left the hour")

    def test_daily_cap_and_total(self):
        self.assertEqual(self.take("alice", 0, per_day=1), "")
        self.assertIn("limit is 1 a day", self.take("alice", 7200, per_day=1))
        self.assertEqual(self.take("bob", 0, day_total=2), "")
        self.assertIn("everyone together", self.take("carol", 10, day_total=2))
        self.assertEqual(self.take("carol", 86401, day_total=2), "", "uses older than a day are dropped")

    def test_refusal_counts_nothing_and_give_back_uncounts(self):
        self.take("alice", 0, per_hour=1)
        self.take("alice", 10, per_hour=1)
        usagecap.give_back("t", "alice")
        self.assertEqual(usagecap.take("t", "alice", per_hour=1), "")

    def test_no_caps_never_refuses(self):
        for i in range(30):
            self.assertEqual(self.take("", i), "")


if __name__ == "__main__":
    unittest.main()
