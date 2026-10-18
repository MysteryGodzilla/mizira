#!/usr/bin/env python3
# Copyright (C) 2026 MysteryGodzilla
# Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
# SPDX-License-Identifier: GPL-3.0-only

import os
import sys
import tempfile
import unittest
from unittest.mock import patch

sys.path[:0] = [os.path.join(os.path.dirname(__file__), ".."), os.path.join(os.path.dirname(__file__), "..", "lib")]

import imagegen
from metald_tools import comfyui, runpod, usagecap


class RunPodImageTests(unittest.TestCase):
    def setUp(self):
        d = tempfile.TemporaryDirectory()
        self.addCleanup(d.cleanup)
        for p in (patch.dict(os.environ, {"METALD_DATADIR": d.name}),
                  patch.object(imagegen, "BACKEND", "runpod"),
                  patch.object(imagegen, "SAFETY_CHECK", "encoder"),
                  patch.object(imagegen, "REVIEW_PROMPTS", False),
                  patch.object(imagegen, "PER_HOUR", 1),
                  patch.object(imagegen, "refine_prompt", lambda p: p),
                  patch.object(imagegen, "upload_image", lambda b: "https://files.example.com/a.png")):
            p.start()
            self.addCleanup(p.stop)

    def draw(self, verdict="SAFE - a cat", error=None, who="alice"):
        def run(workflow, timeout, log=None):
            self.sent = workflow
            if error:
                raise error
            return {"images": [b"png"], "texts": [verdict] if verdict is not None else []}
        with patch.object(runpod, "run", run):
            return imagegen.generate_image("a cat", 1024, 1024, requested_by=who)

    def test_encoder_check_rides_along_and_passes(self):
        self.assertEqual(self.draw(), "url: https://files.example.com/a.png")
        self.assertEqual(self.sent["9"]["class_type"], "TextGenerate")
        self.assertEqual(self.sent["9"]["inputs"]["image"], ["7", 0])
        self.assertEqual(self.sent["10"], {"class_type": "PreviewAny", "inputs": {"source": ["9", 0]}})

    def test_unsafe_or_missing_verdict_is_not_posted(self):
        for verdict in ("UNSAFE - gore", "", None):
            with patch.object(imagegen, "PER_HOUR", 0):
                reply = self.draw(verdict)
            self.assertTrue(reply.startswith("Error: generated image did not pass"), (verdict, reply))

    def test_cap_counts_only_posted_pictures(self):
        self.assertTrue(self.draw(error=comfyui.ComfyError("generation failed")).startswith("Error:"))
        self.assertTrue(self.draw().startswith("url: "), "a failed job gave its use back")
        self.assertTrue(self.draw().startswith("Limit reached:"))
        self.assertTrue(self.draw(who="bob").startswith("url: "))

    def test_timeout_says_the_server_is_starting(self):
        self.assertEqual(self.draw(error=comfyui.ComfyError("generation timed out")), imagegen.COLD_REPLY)

    def test_encoder_check_needs_runpod(self):
        with patch.object(imagegen, "BACKEND", "comfyui"):
            self.assertIn("needs IMAGE_GEN_BACKEND runpod", imagegen.generate_image("a cat", 1024, 1024))

    def test_character_requests_get_an_offer_not_a_refusal(self):
        with patch.object(imagegen, "REVIEW_PROMPTS", True), \
                patch.object(imagegen.safetyreview, "review", lambda *a, **k: (False, "character Mallorymon")):
            reply = self.draw()
        self.assertTrue(reply.startswith("Not drawn: Mallorymon"), reply)
        self.assertFalse(reply.lower().startswith("error: refused"))
        with patch.object(imagegen, "REVIEW_PROMPTS", True), \
                patch.object(imagegen.safetyreview, "review", lambda *a, **k: (False, "sexual content")):
            self.assertTrue(self.draw().startswith("Error: refused"))
        self.assertTrue(self.draw().startswith("url: "), "refused requests don't use up the cap")

    def test_announcement_warns_when_cold(self):
        with patch.object(runpod, "cold", lambda: True):
            self.assertEqual(imagegen.announcement(), "action: " + imagegen.ANNOUNCE_COLD)
        with patch.object(runpod, "cold", lambda: False):
            self.assertEqual(imagegen.announcement(), "action: " + imagegen.ANNOUNCE)

    def test_sizes_are_bounded(self):
        self.assertEqual([imagegen.side(v) for v in (None, "x", 100, 1000, 9999)], [1024, 1024, 512, 992, 1536])


if __name__ == "__main__":
    unittest.main()
