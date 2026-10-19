#!/usr/bin/env python3
# Copyright (C) 2026 MysteryGodzilla
# Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
# SPDX-License-Identifier: GPL-3.0-only

import base64
import json
import os
import sys
import unittest
from unittest.mock import patch

sys.path[:0] = [os.path.join(os.path.dirname(__file__), ".."), os.path.join(os.path.dirname(__file__), "..", "lib")]

from fakeserver import FakeServer
from metald_tools import comfyui, runpod

PNG = b"\x89PNG fake"


class RunPodTests(unittest.TestCase):
    def serve(self, routes):
        srv = FakeServer(routes)
        self.addCleanup(srv.close)
        env = {"RUNPOD_API_KEY": "k1", "RUNPOD_ENDPOINT_ID": "ep1"}
        p = patch.dict(os.environ, env)
        p.start()
        self.addCleanup(p.stop)
        for name, value in (("API", srv.url), ("POLL_INTERVAL", 0)):
            q = patch.object(runpod, name, value)
            q.start()
            self.addCleanup(q.stop)
        return srv

    def test_run_returns_images_and_texts(self):
        done = {"status": "COMPLETED", "delayTime": 900, "executionTime": 8100,
                "output": {"images": [{"type": "base64", "data": base64.b64encode(PNG).decode()}],
                           "texts": ["SAFE - a cat"]}}
        srv = self.serve({"/ep1/run": lambda p, b: (200, {"id": "j1", "status": "IN_QUEUE"}),
                          "/ep1/status/j1": lambda p, b: (200, done)})
        out = runpod.run({"1": {}}, timeout=5, log=lambda m: None)
        self.assertEqual(out, {"images": [PNG], "texts": ["SAFE - a cat"]})
        sent = srv.seen[0]
        self.assertEqual(json.loads(sent["body"]), {"input": {"workflow": {"1": {}}}})
        self.assertEqual(sent["headers"]["authorization"], "Bearer k1")
        self.assertEqual(sent["headers"]["user-agent"], runpod.USER_AGENT)

    def test_inputs_go_with_the_job(self):
        done = {"status": "COMPLETED", "output": {"images": [{"data": base64.b64encode(PNG).decode()}]}}
        srv = self.serve({"/ep1/run": lambda p, b: (200, {"id": "j1"}),
                          "/ep1/status/j1": lambda p, b: (200, done)})
        runpod.run({"1": {}}, timeout=5, log=lambda m: None, inputs={"src.png": b"abc"})
        sent = json.loads(srv.seen[0]["body"])["input"]
        self.assertEqual(sent["images"], [{"name": "src.png", "image": base64.b64encode(b"abc").decode()}])

    def test_failed_job_is_a_generic_error(self):
        self.serve({"/ep1/run": lambda p, b: (200, {"id": "j1"}),
                    "/ep1/status/j1": lambda p, b: (200, {"status": "FAILED", "error": "secret detail"})})
        with self.assertRaises(comfyui.ComfyError) as e:
            runpod.run({}, timeout=5, log=lambda m: None)
        self.assertEqual(str(e.exception), "generation failed")

    def test_timeout_cancels_the_job(self):
        srv = self.serve({"/ep1/run": lambda p, b: (200, {"id": "j1"}),
                          "/ep1/status/j1": lambda p, b: (200, {"status": "IN_QUEUE"}),
                          "/ep1/cancel/j1": lambda p, b: (200, {"status": "CANCELLED"})})
        with self.assertRaises(comfyui.ComfyError) as e:
            runpod.run({}, timeout=0.2, log=lambda m: None)
        self.assertEqual(str(e.exception), "generation timed out")
        self.assertTrue(any(s["path"] == "/ep1/cancel/j1" for s in srv.seen))

    def test_unreachable(self):
        with patch.object(runpod, "API", "http://127.0.0.1:1"):
            with self.assertRaises(comfyui.ComfyError) as e:
                runpod.run({}, timeout=1, log=lambda m: None)
        self.assertEqual(str(e.exception), "backend unreachable")

    def test_cold_unless_a_worker_is_running(self):
        for workers, cold in (({"idle": 0, "running": 0, "ready": 0, "initializing": 1}, True),
                              ({"idle": 1, "running": 0, "ready": 1}, True),
                              ({"running": 1, "idle": 1}, False)):
            self.serve({"/ep1/health": lambda p, b, w=workers: (200, {"workers": w})})
            self.assertEqual(runpod.cold(), cold, workers)

    def test_unknown_health_is_not_cold(self):
        with patch.object(runpod, "API", "http://127.0.0.1:1"):
            self.assertFalse(runpod.cold())


if __name__ == "__main__":
    unittest.main()
