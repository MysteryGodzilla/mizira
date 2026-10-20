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

# The tests below swap refine_prompt out; this is the real one.
REFINE_PROMPT = imagegen.refine_prompt


class RunPodImageTests(unittest.TestCase):
    def setUp(self):
        d = tempfile.TemporaryDirectory()
        self.addCleanup(d.cleanup)
        for p in (patch.dict(os.environ, {"METALD_DATADIR": d.name}),
                  patch.object(imagegen, "BACKEND", "runpod"),
                  patch.object(imagegen, "SAFETY_CHECK", "encoder"),
                  patch.object(imagegen, "REVIEW_PROMPTS", False),
                  patch.object(imagegen, "PER_HOUR", 1),
                  patch.object(imagegen, "refine_prompt", lambda p, people="": p),
                  patch.object(imagegen, "upload_image", lambda b: "https://files.example.com/a.png")):
            p.start()
            self.addCleanup(p.stop)

    def draw(self, verdict="SAFE - a cat", error=None, who="alice", admin=False):
        def run(workflow, timeout, log=None, inputs=None):
            self.sent = workflow
            if error:
                raise error
            return {"images": [b"png"], "texts": [verdict] if verdict is not None else []}
        with patch.object(runpod, "run", run):
            return imagegen.generate_image("a cat", 1024, 1024, requested_by=who, admin=admin)

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

    def test_admins_are_not_capped_or_counted(self):
        with patch.object(imagegen, "DAY_TOTAL", 1):
            for _ in range(3):
                self.assertTrue(self.draw(who="carol", admin=True).startswith("url: "))
            self.assertTrue(self.draw().startswith("url: "), "admin pictures used up the day total")

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
            self.assertEqual(imagegen.announcement({"prompt": "a cat"}), "action: " + imagegen.ANNOUNCE)
            self.assertEqual(imagegen.announcement({"prompt": "pink hair", "image_url": "https://files.example.com/a.png"}),
                             "action: " + imagegen.ANNOUNCE_EDIT)
            self.assertEqual(imagegen.announcement({"image_url": " "}), "action: " + imagegen.ANNOUNCE)

    def test_sizes_are_bounded(self):
        self.assertEqual([imagegen.side(v) for v in (None, "x", 100, 1000, 9999)], [1024, 1024, 512, 992, 1536])

    def test_edit_sends_the_picture_and_keeps_its_size(self):
        seen = {}

        def run(workflow, timeout, log=None, inputs=None):
            seen.update(workflow=workflow, inputs=inputs)
            return {"images": [b"png"], "texts": ["SAFE"]}
        with patch.object(imagegen, "EDIT_HOSTS", ["files.example.com"]), \
                patch.object(imagegen.vision, "fetch_image", lambda url: (b"source", "image/png")), \
                patch.object(imagegen.vision, "fit_for_model", lambda b, m: (b, m)), \
                patch.object(imagegen, "refine_prompt", lambda p, people="": self.fail("an edit was sent to the refiner")), \
                patch.object(runpod, "run", run):
            reply = imagegen.generate_image("make her hair pink", 1024, 1024, requested_by="alice",
                                            image_url="https://files.example.com/a.png")
        self.assertEqual(reply, "url: https://files.example.com/a.png")
        wf = seen["workflow"]
        self.assertEqual(seen["inputs"], {imagegen.SOURCE_NAME: b"source"})
        self.assertEqual(wf["11"], {"class_type": "LoadImage", "inputs": {"image": imagegen.SOURCE_NAME}})
        self.assertEqual(wf["4"]["inputs"]["images.image_1"], ["11", 0])
        self.assertEqual(wf["4"]["inputs"]["prompt"], "make her hair pink. Keep everything else exactly the same.")
        self.assertEqual(wf["6"]["inputs"]["latent_image"], ["4", 2])
        self.assertNotIn("5", wf)

    def test_edit_only_from_allowed_hosts(self):
        with patch.object(imagegen, "EDIT_HOSTS", ["files.example.com"]), \
                patch.object(imagegen.vision, "fetch_image", lambda url: self.fail("fetched a disallowed host")):
            reply = imagegen.generate_image("pink hair", 1024, 1024, requested_by="alice",
                                            image_url="https://evil.example.net/a.png")
        self.assertTrue(reply.startswith("Not edited: only pictures on files.example.com"), reply)
        self.assertTrue(self.draw().startswith("url: "), "a refused edit doesn't use up the cap")

    def test_schema_keeps_other_bots_out(self):
        import io, json
        from contextlib import redirect_stdout
        buf = io.StringIO()
        with redirect_stdout(buf):
            imagegen.print_schema()
        schema = json.loads(buf.getvalue())
        self.assertTrue(schema["nobots"] and schema["once"] and schema["people"] and schema["requester"] and schema["announcer"])
        self.assertIn("image_url", schema["properties"])

    def edit(self, checks):
        """An edit whose encoder checks answer checks in turn; returns the reply and the workflows sent."""
        sent = []

        def run(workflow, timeout, log=None, inputs=None):
            sent.append(workflow)
            return {"images": [b"png%d" % len(sent)], "texts": ["SAFE", checks[len(sent) - 1]]}
        with patch.object(imagegen, "EDIT_HOSTS", ["files.example.com"]), \
                patch.object(imagegen, "EDIT_CHECK", "Does it show: {change}? YES or NO"), \
                patch.object(imagegen.vision, "fetch_image", lambda url: (b"source", "image/png")), \
                patch.object(imagegen.vision, "fit_for_model", lambda b, m: (b, m)), \
                patch.object(imagegen, "upload_image", lambda b: "url-of-" + b.decode()), \
                patch.object(runpod, "run", run):
            reply = imagegen.generate_image("make her hair pink", 1024, 1024, requested_by="alice",
                                            image_url="https://files.example.com/a.png")
        return reply, sent

    def test_missed_edit_is_tried_once_more(self):
        reply, sent = self.edit(["NO", "YES"])
        self.assertEqual(reply, "url: url-of-png2")
        self.assertEqual(len(sent), 2)
        self.assertEqual(sent[0]["12"]["inputs"]["prompt"], "Does it show: make her hair pink? YES or NO")
        with patch.object(imagegen, "PER_HOUR", 0):
            reply, sent = self.edit(["NO", "NO"])
        self.assertEqual((reply, len(sent)), ("url: url-of-png2", 2), "only one retry")
        with patch.object(imagegen, "PER_HOUR", 0):
            reply, sent = self.edit(["YES"])
        self.assertEqual((reply, len(sent)), ("url: url-of-png1", 1))

    def test_people_reach_the_prompt_writer(self):
        seen = []
        with patch.object(imagegen, "PEOPLE_FRAME", "People: {people}"), \
                patch.object(imagegen, "_refine", lambda text, *a: seen.append(text) or text):
            REFINE_PROMPT("bob at the beach", "bob: drawn as female")
            REFINE_PROMPT("a cat", "")
        self.assertEqual(seen, ["bob at the beach\n\nPeople: bob: drawn as female", "a cat"])


if __name__ == "__main__":
    unittest.main()
