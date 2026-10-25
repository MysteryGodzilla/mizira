#!/usr/bin/env python3
# Copyright (C) 2026 MysteryGodzilla
# Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
# SPDX-License-Identifier: GPL-3.0-only

import json
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
                  patch.object(imagegen, "refine_prompt", lambda p, people="", style=None: p),
                  patch.object(imagegen, "upload_image", lambda b: "https://files.example.com/a.png")):
            p.start()
            self.addCleanup(p.stop)

    def draw(self, verdict="SAFE - a cat", error=None, who="alice", admin=False):
        def run(workflow, timeout, log=None, inputs=None, loras=None):
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

        def run(workflow, timeout, log=None, inputs=None, loras=None):
            seen.update(workflow=workflow, inputs=inputs)
            return {"images": [b"png"], "texts": ["SAFE"]}
        with patch.object(imagegen, "EDIT_HOSTS", ["files.example.com"]), \
                patch.object(imagegen.vision, "fetch_image", lambda url: (b"source", "image/png")), \
                patch.object(imagegen.vision, "fit_for_model", lambda b, m: (b, m)), \
                patch.object(imagegen, "refine_prompt", lambda p, people="", style=None: self.fail("an edit was sent to the refiner")), \
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

    STYLES = ("soyuz | draw | 0.9 | soyuzmult | Soviet cartoon | https://civitai.com/api/download/models/1 | " + "a" * 64 + "\n"
              "# a comment\n"
              "clay | edit | 1 | Transform the image into a clay sculpture style | clay figure | "
              "https://huggingface.co/x/y/resolve/abc/clay.safetensors | " + "B" * 64 + "\n"
              "bad line without fields\n"
              "huge | draw | 9 | | too strong | https://civitai.com/x | " + "c" * 64)

    def test_styles_parse_and_skip_bad_lines(self):
        styles = imagegen.parse_styles(self.STYLES)
        self.assertEqual(sorted(styles), ["clay", "soyuz"])
        self.assertEqual((styles["soyuz"]["kind"], styles["soyuz"]["strength"], styles["soyuz"]["trigger"]),
                         ("draw", 0.9, "soyuzmult"))
        self.assertEqual(imagegen.style_lora(styles["clay"])["name"], "style-clay-bbbbbbbbbbbb.safetensors")

    def test_style_cfg_overrides_the_usual(self):
        # The PC-98 LoRA at cfg 1 drew retro anime without its palette or dithering; it needs 2.5.
        styles = imagegen.parse_styles(
            "pc98 | draw | 0.8 cfg 2.5 | pc98dither | PC-98 art | https://files.example.com/p.safetensors | " + "d" * 64 + "\n"
            "wild | draw | 1 cfg 40 | | too much | https://civitai.com/x | " + "e" * 64 + "\n" + self.STYLES)
        self.assertEqual(sorted(styles), ["clay", "pc98", "soyuz"])
        self.assertEqual((styles["pc98"]["strength"], styles["pc98"]["cfg"], styles["soyuz"]["cfg"]), (0.8, 2.5, 0))
        with patch.object(imagegen, "GEN_CFG", 1.0), patch.object(imagegen, "NEGATIVE_CFG", 1.5):
            cfg = lambda style, negative="": imagegen.build_workflow("a fox", 1024, 1024, negative, style=style)["prompt"]["6"]["inputs"]["cfg"]
            self.assertEqual([cfg(styles["pc98"]), cfg(styles["pc98"], "text"), cfg(styles["soyuz"]), cfg(styles["soyuz"], "text")],
                             [2.5, 2.5, 1.0, 1.5])

    def styled(self, style, image_url=""):
        sent = {}

        def run(workflow, timeout, log=None, inputs=None, loras=None):
            sent.update(workflow=workflow, loras=loras)
            return {"images": [b"png"], "texts": ["SAFE", "YES"]}
        with patch.object(imagegen, "STYLES", imagegen.parse_styles(self.STYLES)), \
                patch.object(imagegen, "STYLE_FRAME", "Art style: {style}"), \
                patch.object(imagegen, "refine_prompt", lambda p, people="", style=None:
                             p + (" / " + style["description"] if style else "")), \
                patch.object(imagegen, "EDIT_HOSTS", ["files.example.com"]), \
                patch.object(imagegen.vision, "fetch_image", lambda url: (b"source", "image/png")), \
                patch.object(imagegen.vision, "fit_for_model", lambda b, m: (b, m)), \
                patch.object(imagegen, "PER_HOUR", 0), \
                patch.object(runpod, "run", run):
            reply = imagegen.generate_image("a wolf chasing a hare", 1024, 1024, requested_by="alice",
                                            image_url=image_url, style=style)
        return reply, sent

    def test_draw_style_adds_its_lora_and_trigger(self):
        reply, sent = self.styled("soyuz")
        self.assertTrue(reply.startswith("url: "), reply)
        wf = sent["workflow"]
        self.assertEqual(wf["20"]["inputs"], {"model": ["1", 0], "lora_name": "style-soyuz-aaaaaaaaaaaa.safetensors",
                                              "strength_model": 0.9})
        self.assertEqual(wf["6"]["inputs"]["model"], ["20", 0])
        self.assertEqual(wf["4"]["inputs"]["prompt"], "soyuzmult, a wolf chasing a hare / Soviet cartoon")
        self.assertEqual(sent["loras"], [{"name": "style-soyuz-aaaaaaaaaaaa.safetensors",
                                          "url": "https://civitai.com/api/download/models/1", "sha256": "a" * 64}])
        reply, sent = self.styled("")
        self.assertNotIn("20", sent["workflow"])
        self.assertIsNone(sent["loras"])

    def test_edit_style_is_its_own_instruction(self):
        reply, sent = self.styled("clay", "https://files.example.com/a.png")
        self.assertTrue(reply.startswith("url: "), reply)
        self.assertEqual(sent["workflow"]["4"]["inputs"]["prompt"],
                         "Transform the image into a clay sculpture style. Keep everything else exactly the same.")
        self.assertEqual(sent["loras"][0]["sha256"], "b" * 64)

    def test_styles_are_kept_to_their_kind(self):
        for style, url, start in (("clay", "", "Not done: clay restyles an existing picture"),
                                  ("vaporwave", "", "Not done: there's no style called 'vaporwave'. Styles: clay, soyuz")):
            reply, sent = self.styled(style, url)
            self.assertTrue(reply.startswith(start), reply)
            self.assertEqual(sent, {}, "nothing was rendered")
        self.assertTrue(self.draw().startswith("url: "), "a refused style doesn't use up the cap")

    def test_draw_style_on_an_edit_is_dropped(self):
        # The model carried "painterly" from drawing a picture onto editing it, and every edit was refused.
        reply, sent = self.styled("soyuz", "https://files.example.com/a.png")
        self.assertTrue(reply.startswith("url: "), reply)
        self.assertNotIn("20", sent["workflow"])
        self.assertIsNone(sent["loras"])

    GUIDED = ("ck3 | draw | 1 | ckiiistyle | CK3 portrait | https://civitai.com/x | " + "a" * 64 +
              " | write it as: a 3d render of [subject], painterly backdrop\n"
              "angle | edit | 1 | <qma> {prompt} | camera moved | https://huggingface.co/x | " + "b" * 64 +
              " | prompt: '<view> <height> <distance>', e.g. 'back view elevated shot wide shot'\n"
              "outfit | edit2 | 1 | Dress the person in image 1 in the {prompt} shown in image 2. Keep their face. | "
              "outfit swap | https://huggingface.co/y | " + "c" * 64)

    def guided(self, style, prompt="a knight", image_url="", reference_url="", admin=False):
        sent = {}

        def run(workflow, timeout, log=None, inputs=None, loras=None):
            sent.update(workflow=workflow, inputs=inputs)
            return {"images": [b"png"], "texts": ["SAFE"]}
        refined = []
        with patch.object(imagegen, "STYLES", imagegen.parse_styles(self.GUIDED)), \
                patch.object(imagegen, "STYLE_FRAME", "Art style: {style}"), \
                patch.object(imagegen, "REFERENCE_PROMPT", "What clothes?"), \
                patch.object(imagegen, "EDIT_HOSTS", ["files.example.com"]), \
                patch.object(imagegen, "EDIT_CHECK", "Does it show: {change}?"), \
                patch.object(imagegen.vision, "fetch_image", lambda url: (url.encode(), "image/png")), \
                patch.object(imagegen.vision, "fit_for_model", lambda b, m: (b, m)), \
                patch.object(imagegen.chat, "endpoint", lambda name: {"url": "", "model": "", "key": ""}), \
                patch.object(imagegen, "_refine", lambda p, *a: refined.append(p) or "refined"), \
                patch.object(imagegen, "refine_prompt", REFINE_PROMPT), \
                patch.object(imagegen, "PER_HOUR", 0), \
                patch.object(runpod, "run", run):
            reply = imagegen.generate_image(prompt, 1024, 1024, requested_by="alice", image_url=image_url,
                                            style=style, reference_url=reference_url, admin=admin)
        return reply, sent, refined

    def test_draw_guide_goes_to_the_prompt_writer(self):
        reply, sent, refined = self.guided("ck3")
        self.assertTrue(reply.startswith("url: "), reply)
        self.assertIn("Art style: CK3 portrait. write it as: a 3d render of [subject], painterly backdrop", refined[0])
        self.assertEqual(sent["workflow"]["4"]["inputs"]["prompt"], "ckiiistyle, refined")

    def test_edit_trigger_takes_the_request(self):
        reply, sent, _ = self.guided("angle", "back view elevated shot wide shot.", "https://files.example.com/a.png")
        self.assertTrue(reply.startswith("url: "), reply)
        wf = sent["workflow"]
        self.assertEqual(wf["4"]["inputs"]["prompt"], "<qma> back view elevated shot wide shot")
        self.assertNotIn("12", wf, "a camera move isn't edit-checked")

    def test_second_picture_is_for_admins_and_reaches_image_2(self):
        args = ("outfit", "red leather jacket", "https://files.example.com/a.png", "https://shop.example.net/jacket.jpg")
        reply, sent, _ = self.guided(*args)
        self.assertTrue(reply.startswith("Not allowed: outfit is for admins only"), reply)
        self.assertEqual(sent, {})
        reply, sent, _ = self.guided(*args, admin=True)
        self.assertTrue(reply.startswith("url: "), reply)
        wf = sent["workflow"]
        self.assertEqual(wf["4"]["inputs"]["prompt"], ["17", 0])
        self.assertEqual(wf["14"], {"class_type": "LoadImage", "inputs": {"image": imagegen.REFERENCE_NAME}})
        self.assertEqual(wf["4"]["inputs"]["images.image_2"], ["14", 0])
        self.assertEqual(sent["inputs"], {imagegen.SOURCE_NAME: b"https://files.example.com/a.png",
                                          imagegen.REFERENCE_NAME: b"https://shop.example.net/jacket.jpg"})
        for style, ref, start in (("outfit", "", "Not done: outfit needs a second picture"),
                                  ("angle", "https://x.example.net/b.png", "Not done: angle takes one picture"),
                                  ("", "https://x.example.net/b.png", "Not done: reference_url only goes")):
            reply, _, _ = self.guided(style, "x", "https://files.example.com/a.png", ref, admin=True)
            self.assertTrue(reply.startswith(start), reply)

    def test_unnamed_outfit_is_described_from_the_second_picture(self):
        args = ("https://files.example.com/a.png", "https://shop.example.net/jacket.jpg")
        reply, sent, _ = self.guided("outfit", "the outfit in that picture", *args, admin=True)
        self.assertTrue(reply.startswith("url: "), reply)
        wf = sent["workflow"]
        self.assertEqual(wf["15"]["inputs"]["image"], ["14", 0])
        self.assertEqual(wf["15"]["inputs"]["prompt"], "What clothes?")
        self.assertEqual(wf["17"]["inputs"]["string"],
                         "Dress the person in image 1 in the {prompt} shown in image 2. Keep their face.")
        self.assertEqual(wf["4"]["inputs"]["prompt"], ["17", 0])
        # Clothes the chat model names are a guess: it never sees the second picture.
        reply, sent, _ = self.guided("outfit", "a lavender knit cardigan and pleated mini skirt", *args, admin=True)
        self.assertEqual(sent["workflow"]["15"]["inputs"]["image"], ["14", 0])
        self.assertTrue(imagegen.says_nothing("put her in that outfit from image 2"))
        # As the model wrote it live: a whole sentence, still no clothes named.
        self.assertTrue(imagegen.says_nothing("The girl with short black hair from the first image is now wearing "
                                              "the outfit from the second image. She is still standing on the street."))
        self.assertFalse(imagegen.says_nothing("the green coat"))
        self.assertFalse(imagegen.says_nothing("a t-shirt and jeans"))

    def test_schema_lists_the_styles(self):
        import io, json
        from contextlib import redirect_stdout
        with patch.object(imagegen, "STYLES", imagegen.parse_styles(self.STYLES)):
            buf = io.StringIO()
            with redirect_stdout(buf):
                imagegen.print_schema()
        style = json.loads(buf.getvalue())["properties"]["style"]
        self.assertEqual(style["enum"], ["clay", "soyuz"])
        self.assertIn("soyuz (Soviet cartoon)", style["description"])
        self.assertIn("restyle an earlier picture (with its image_url): clay (clay figure)", style["description"])

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

        def run(workflow, timeout, log=None, inputs=None, loras=None):
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
            REFINE_PROMPT("bob at the beach", '[{"name": "bob", "drawn_as": "female", "notes": ["silver hair"]}]')
            REFINE_PROMPT("a cat", "")
            REFINE_PROMPT("a dog", "not json")
        self.assertEqual(seen, ["bob at the beach\n\nPeople: bob: drawn as female; silver hair", "a cat", "a dog"])

    def test_set_looks_go_in_as_they_are(self):
        # The prompt writer gave a woman glasses she doesn't wear and drew adults as children: someone
        # with a profile look is only named to it, and their look is added to the final prompt as set.
        people = ('[{"name": "hori", "drawn_as": "female", "look": "adult woman, green eyes, short lavender bob",'
                  ' "outfit": "charcoal blazer", "variant": "business"},'
                  ' {"name": "remy", "drawn_as": "male", "notes": ["grey rat with stubble"]}]')
        seen, sent = [], {}

        def run(workflow, timeout, log=None, inputs=None, loras=None):
            sent.update(workflow=workflow)
            return {"images": [b"png"], "texts": ["SAFE"]}
        with patch.object(imagegen, "PEOPLE_FRAME", "People: {people}"), \
                patch.object(imagegen, "CHARACTER_FRAME", "Characters: {characters}"), \
                patch.object(imagegen, "refine_prompt", REFINE_PROMPT), \
                patch.object(imagegen.chat, "endpoint", lambda name: {"url": "", "model": "", "key": ""}), \
                patch.object(imagegen, "_refine", lambda text, *a: seen.append(text) or "hori and remy at a desk"), \
                patch.object(runpod, "run", run):
            reply = imagegen.generate_image("hori and remy at a desk", 1024, 1024, requested_by="alice", people=people)
        self.assertTrue(reply.startswith("url: "), reply)
        # "drawn as": "MysteryGodzilla" in a dinosaur onesie was drawn as a reptilian "he".
        self.assertEqual(seen[0], "hori and remy at a desk\n\nPeople: hori - outfit: charcoal blazer; drawn as female\n"
                                  "remy: drawn as male; grey rat with stubble")
        # Next to her name, not as a list: listed apart, it drew a second woman beside the one described.
        self.assertEqual(sent["workflow"]["4"]["inputs"]["prompt"],
                         "hori (adult woman, green eyes, short lavender bob) and remy at a desk")
        self.assertEqual(imagegen.bind_looks("Hori-chan and HORI", imagegen.parse_people(people))[0],
                         "Hori-chan and HORI (adult woman, green eyes, short lavender bob)")

    def test_clothes_nobody_asked_for_are_left_out(self):
        # The chat model dressed baremetal in "a modern tech outfit" when her sysadmin alt was asked for,
        # and the prompt writer drew a second woman in the alt's clothes.
        alt = [{"name": "baremetal", "look": "adult woman", "outfit": "a white lab coat", "variant": "sysadmin"}]
        made_up = ("baremetal (adult woman mid-20s) with very long blue hair. She's wearing a modern tech outfit: "
                   "dark blue fitted turtleneck sweater, grey cargo pants, white sneakers. She stands in a server room.")
        for prompt, request, people, want in [
            (made_up, "mizira new image of sysadmin baremetal looking at servers", alt,
             "baremetal (adult woman mid-20s) with very long blue hair. She stands in a server room."),
            ("hori wearing a red dress, at the top of the steps, with sandals", "draw hori in a red dress on the steps",
             [{"name": "hori", "look": "adult woman", "outfit": "a white t-shirt"}],
             "hori wearing a red dress, at the top of the steps"),
            (made_up, "", alt, made_up),
            ("alice streetwear look with tech print, side buns peeking out from hood, holding a console",
             "draw alice streetwear look", [{"name": "alice", "look": "adult woman", "outfit": "a lavender hoodie"}],
             "alice streetwear look with tech print, holding a console"),
            (made_up, "draw remy", [{"name": "remy", "notes": ["grey rat"]}], made_up),
        ]:
            self.assertEqual(imagegen.unasked_clothes_dropped(prompt, request, people), want, request)

    def test_request_outfit_replaces_the_set_one(self):
        # "me wearing a baltimore ravens jersey": given her set outfit too, the prompt writer kept her
        # thigh-high socks under the jersey and once drew two people on the couch.
        me = {"name": "alice", "look": "adult woman", "outfit": "a lilac cardigan and pleated skirt"}
        office = dict(me, outfit="a charcoal blazer", variant="office")
        for request, people, outfits in [
            ("draw me wearing a baltimore ravens jersey", [me], [""]),
            ("me wearing a ravens jeresy on the couch", [me], [""]),
            ("draw me in a hat at the beach", [me], [me["outfit"]]),
            ("draw me in my office look in a jersey", [office], [office["outfit"]]),
            ("draw me at a press conference", [me], [me["outfit"]]),
        ]:
            self.assertEqual([p["outfit"] for p in imagegen.outfit_from_request(people, request)], outfits, request)
        self.assertEqual(imagegen.unasked_clothes_dropped("alice on a couch, the jersey fits snugly", "alice in a jeresy",
                                                          [me]), "alice on a couch, the jersey fits snugly")
        # With nothing to say, her line made the prompt writer add "the second person ... from their entry".
        self.assertEqual(imagegen.people_lines(imagegen.outfit_from_request([me], "me in a jersey")), "")
        bob = {"name": "bob", "look": "adult man", "outfit": "a grey hoodie"}
        for request, people, outfits in [
            ("draw yourself in the heroine's outfit from that game", [me], [""]),
            ("draw me in a maid outfit", [me], [""]),
            ("draw me in my usual outfit", [me], [me["outfit"]]),
            ("draw me in my office outfit with bob", [office, bob], [office["outfit"], bob["outfit"]]),
        ]:
            self.assertEqual([p["outfit"] for p in imagegen.outfit_from_request(people, request)], outfits, request)

    def test_a_kind_away_from_the_name_is_the_name(self):
        # "bob and dave on a bench ... while capybara man sits beside him" drew a second capybara.
        dave = {"name": "dave", "look": "adult humanoid capybara man, big heavyset build", "outfit": "a black sweater"}
        bob = {"name": "bob", "look": "adult humanoid rat man, slim", "outfit": "a black hoodie", "variant": "street"}
        alice = {"name": "alice", "look": "adult woman, green eyes", "outfit": "a white t-shirt"}
        for prompt, want in [
            ("bob and dave on a bench, humanoid rat man leaning back while capybara man sits beside him",
             "bob and dave on a bench, bob leaning back while dave sits beside him"),
            ("dave, a capybara man, waving at bob the humanoid rat", "dave, a capybara man, waving at bob the humanoid rat"),
            ("bob feeding a baby capybara", "bob feeding a baby capybara"),
            ("a woman and a rat man chef in an alley", "a woman and bob chef in an alley"),
        ]:
            self.assertEqual(imagegen.kind_named(prompt, [dave, bob, alice]), want, prompt)

    def test_set_looks_lose_their_brackets(self):
        # "alice (a slender elf-eared woman ... in an oversized white tee)": her look is added as set,
        # and the tee went on a second alice.
        alice = {"name": "alice", "look": "adult woman", "outfit": "a purple gown", "variant": "villainess"}
        carol = {"name": "carol", "notes": ["red hair"]}
        for prompt, want in [
            ("alice (slender elf-eared woman, two-tone hair, in an oversized white tee) at night",
             "alice at night"),
            ("carol (red-haired woman) waves at alice (the villainess)", "carol (red-haired woman) waves at alice"),
            # Never closed: the bracket ran on into the next sentence.
            ("alice (slender woman, leaning back. Beside her stands carol (the guard), arms crossed",
             "alice (slender woman, leaning back. Beside her stands carol (the guard), arms crossed"),
        ]:
            self.assertEqual(imagegen.brackets_dropped(prompt, [alice, carol]), want, prompt)

    def test_clothes_off_an_alt_are_left_out(self):
        # With her villainess alt, the chat model also put her in her usual tee and shorts; a second
        # alice was drawn in them.
        alice = {"name": "alice", "look": "adult woman", "outfit": "a flowing deep purple gown, sheer sleeves",
                 "variant": "villainess"}
        bob = {"name": "bob", "look": "adult rat man", "outfit": "a white chef's jacket and apron", "variant": "chef"}
        for prompt, request, want in [
            ("bob next to alice as villainess, alice smirking in an oversized white tee and grey shorts with a bob",
             "draw bob next to villianess alice",
             "bob next to alice as villainess, alice smirking with a bob"),
            # The alt's own outfit goes to the prompt writer as set; the chat model's wording of it only
            # varies it ("her race suit in mint green").
            ("bob crouched in his chef's jacket and apron, alice in her purple gown", "bob and alice",
             "bob crouched, alice"),
            ("alice in a mint green racing suit with purple trim on a motorbike", "alice in her race outfit",
             "alice with purple trim on a motorbike"),
            ("alice wearing a red dress at a party", "alice in her villainess look but in a red dress",
             "alice wearing a red dress at a party"),
            ("carol in jeans waving, alice in the kitchen", "draw carol and alice", "carol in jeans waving, alice in the kitchen"),
        ]:
            self.assertEqual(imagegen.worn_dropped(prompt, request, [alice, bob]), want, prompt)
        # She described the usual robe clause by clause; a second figure wore the alt.
        carol = {"name": "carol", "look": "adult woman", "outfit": "a cream sweater and navy skirt", "variant": "weekend",
                 "usual": ("a flowing white robe with wide bell sleeves trimmed in gold, a deep blue panel hanging down "
                           "the front from a V-neck collar, golden feathered shoulder pieces, an ornate gold brooch")}
        self.assertEqual(imagegen.worn_dropped(
            "carol with wide bell sleeves trimmed in gold, deep blue panel hanging down front from V-neck collar, "
            "golden feathered shoulder pieces, reading under a deep blue night sky, bob waving", "carol and bob",
            [carol, bob]), "carol, reading under a deep blue night sky, bob waving")

    def test_prompt_outfit_replaces_the_set_one(self):
        # Asked a message earlier for a game-style dress, the chat model wrote it into the prompt, the
        # request ("draw the picture") named no clothes, and her set outfit was drawn on a second woman.
        alice = {"name": "alice", "look": "adult woman", "outfit": "a loose white t-shirt and grey shorts"}
        bob = {"name": "bob", "look": "adult man", "outfit": "a grey hoodie"}
        rocker = dict(bob, outfit="a black leather jacket", variant="rocker")
        summer = dict(bob, outfit="a sun hat and a linen shirt", variant="summer")
        for prompt, people, outfits in [
            ("alice in a fantasy game style dress - elegant flowing gown, green eyes", [alice], [""]),
            ("alice in a loose white t-shirt and grey shorts on a couch", [alice], [alice["outfit"]]),
            ("alice and bob in rocker outfits, on stage", [alice, rocker], ["", rocker["outfit"]]),
            ("alice and bob both wearing bob's summer outfit - linen shirts", [alice, summer], ["", summer["outfit"]]),
            ("alice in a red dress next to bob at the park", [alice, bob], ["", bob["outfit"]]),
            ("bob at the park next to alice in a red dress", [alice, bob], ["", bob["outfit"]]),
            ("alice in her usual outfit at the beach", [alice], [alice["outfit"]]),
            ("alice wearing the heroine's spring outfit: layered sleeves", [alice], [""]),
            ("alice on a bridge. The dress is red.", [alice], [alice["outfit"]]),
        ]:
            self.assertEqual([p["outfit"] for p in imagegen.outfit_from_prompt(prompt, people)], outfits, prompt)
        # Asked a message earlier (the bot passes that request on), as a costume, or not at all: only
        # the clothes the asker wanted replace hers.
        for prompt, request, want in [
            ("alice in a fantasy game style dress at the park",
             "draw alice but give her a fantasy game style dress\ndraw the picture",
             "alice in a fantasy game style dress at the park"),
            ("alice in a pirate coat and tricorn hat on a ship", "draw alice as a pirate",
             "alice in a pirate coat and tricorn hat on a ship"),
            ("alice in an oversized hoodie at the park", "you and alice at the park",
             "alice at the park\n\nPeople: alice - outfit: " + alice["outfit"]),
        ]:
            seen = []
            with patch.object(imagegen, "PEOPLE_FRAME", "People: {people}"), \
                    patch.object(imagegen, "refine_prompt", REFINE_PROMPT), \
                    patch.object(imagegen.chat, "endpoint", lambda name: {"url": "", "model": "", "key": ""}), \
                    patch.object(imagegen, "_refine", lambda text, *a: seen.append(text) or "alice in a gown"), \
                    patch.object(imagegen, "PER_HOUR", 0), \
                    patch.object(runpod, "run", lambda *a, **k: {"images": [b"png"], "texts": ["SAFE"]}):
                reply = imagegen.generate_image(prompt, 1024, 1024, requested_by="carol", people=json.dumps([alice]),
                                                request=request)
            self.assertTrue(reply.startswith("url: "), reply)
            self.assertEqual(seen, [want], request)

    def test_prompt_writer_keeps_the_names(self):
        # It wrote "a man ... and a second man" for alice and bob: their looks went on whoever the picture
        # showed, outfits swapped. Asked again once, then the prompt goes as written.
        people = [{"name": "alice", "look": "adult woman", "outfit": "a red race suit", "drawn_as": "female"},
                  {"name": "bob", "look": "adult man", "outfit": "a white robe", "drawn_as": "male"}]
        for answers, want in [
            (["a man on a bike and a second man"], "alice on a bike next to bob. alice wears a red race suit. "
                                                   "bob wears a white robe."),
            (["a man on a bike", "alice on a bike beside bob"], "alice on a bike beside bob"),
        ]:
            replies = iter(answers * 2)
            with patch.object(imagegen, "refine_prompt", lambda p, people="", style=None: next(replies)):
                self.assertEqual(imagegen.refined_with_names("alice on a bike next to bob", people), want)

    def test_failed_prompt_writer_leaves_its_instructions_out(self):
        # A timed-out prompt writer hands back its input, and its instructions and every outfit were drawn.
        people = json.dumps([{"name": "alice", "look": "adult woman", "outfit": "a lilac cardigan"}])
        with patch.object(imagegen, "PEOPLE_FRAME", "People: {people}"), \
                patch.object(imagegen.chat, "endpoint", lambda name: {"url": "", "model": "", "key": ""}), \
                patch.object(imagegen, "_refine", lambda text, *a: text):
            self.assertEqual(REFINE_PROMPT("alice at the beach", people), "alice at the beach. alice wears a lilac cardigan.")
        with patch.object(imagegen, "PEOPLE_FRAME", "People: {people}"), \
                patch.object(imagegen.chat, "endpoint", lambda name: {"url": "", "model": "", "key": ""}), \
                patch.object(imagegen, "_refine", lambda text, *a: "a detailed alice at the beach"):
            self.assertEqual(REFINE_PROMPT("alice at the beach", people), "a detailed alice at the beach")

    def test_group_of_three_or_more_is_named_in_order(self):
        # Five set looks were drawn as four people, two of them fused, until the count and order led the prompt.
        people = [{"name": n} for n in ("carol", "alice", "bob", "dave")]
        frame = "The {count} named people, each drawn once, left to right: {names}."
        with patch.object(imagegen, "GROUP_FRAME", frame):
            self.assertEqual(imagegen.group_line("alice (look) at the bar with bob and Carol by the horizon", people),
                             "The three named people, each drawn once, left to right: alice, bob and carol. ")
            self.assertEqual(imagegen.group_line("alice and bob on a hill", people), "")
            self.assertEqual(imagegen.group_line("alice, bobby and carolina", people), "")

    def test_name_tucked_in_brackets_comes_first(self):
        # "adult woman mid-20s (baremetal) with ..." drew the woman and "another person named baremetal".
        people = [{"name": "baremetal"}]
        for prompt, want in {
            "adult woman mid-20s (baremetal) with long blue hair": "baremetal (adult woman mid-20s) with long blue hair",
            "anime style illustration of adult woman mid-20s (BareMetal) reading":
                "anime style illustration of baremetal (adult woman mid-20s) reading",
            "a woman (baremetal), smiling": "baremetal (woman), smiling",
            "baremetal at a desk": "baremetal at a desk",
            "alice (adult woman) at a desk": "alice (adult woman) at a desk",
        }.items():
            self.assertEqual(imagegen.named_first(prompt, people), want, prompt)

    def test_unnamed_look_goes_on_their_person(self):
        # The prompt writer dropped the name and the look, listed apart, drew a second woman.
        hori = {"name": "hori", "drawn_as": "female", "look": "adult woman, lavender bob"}
        bob = {"name": "bob", "drawn_as": "male", "look": "adult man, red cap"}
        carol = {"name": "carol", "look": "adult, green coat"}
        for prompt, people, want in [
            ("A clean anime illustration of a young adult woman with long hair in a server room", [hori],
             ("A clean anime illustration of a young adult woman named hori (adult woman, lavender bob) with long hair "
              "in a server room", [])),
            ("an old man feeding pigeons", [hori], ("an old man feeding pigeons", [hori])),
            ("a lone figure on a hill", [carol], ("a lone figure named carol (adult, green coat) on a hill", [])),
            ("a woman and a man at a desk", [hori, bob], ("a woman and a man at a desk", [hori, bob])),
            ("a manager at a desk", [bob], ("a manager at a desk", [bob])),
        ]:
            self.assertEqual(imagegen.bind_looks(prompt, people), want, prompt)

    def test_other_names_are_one_person(self):
        # "alice Ally sitting": the look went in after alice and Ally was drawn as a second woman.
        people = [{"name": "alice", "aliases": ["ally", "alice-chan"]}, {"name": "bob"}]
        for prompt, want in {
            "Alice Ally sitting in a cafe": "Alice sitting in a cafe",
            "alice (Ally) and bob waving": "alice and bob waving",
            "Ally in a cafe with alice-chan": "alice in a cafe with alice",
            "alice and Ally's cat": "alice and alice's cat",
            "Rally cars and bob bob": "Rally cars and bob",
        }.items():
            self.assertEqual(imagegen.one_name(prompt, people), want, prompt)

    def test_edit_spells_out_a_set_outfit(self):
        # Told "back in default outfit", the edit model had nothing to go on.
        alice = {"name": "alice", "drawn_as": "female", "look": "adult woman", "outfit": "a cream shirt and lilac cardigan"}
        office = dict(alice, outfit="a charcoal blazer", variant="office")
        bob = {"name": "bob", "drawn_as": "male", "notes": ["grey rat"]}
        put = "Change her outfit to a cream shirt and lilac cardigan. "
        for change, people, want in [
            ("alice back in her default outfit, warm light", [alice], put + "alice back in her default outfit, warm light"),
            ("change her clothes back", [alice], put + "change her clothes back"),
            ("put alice in her office look", [office], "Change her outfit to a charcoal blazer. put alice in her office look"),
            ("alice in a red dress", [alice], "alice in a red dress"),
            ("make it night time", [alice], "make it night time"),
            ("change his outfit to match", [alice, bob], "change his outfit to match"),
            ("alice and bob back in their usual outfits", [alice, bob], put + "alice and bob back in their usual outfits"),
            ("both back in default outfits", [alice, dict(alice, name="carol")], "both back in default outfits"),
        ]:
            self.assertEqual(imagegen.dressed(change, people), want, change)

        sent = {}

        def run(workflow, timeout, log=None, inputs=None, loras=None):
            sent.update(workflow=workflow)
            return {"images": [b"png"], "texts": ["SAFE", "YES"]}
        with patch.object(imagegen, "EDIT_HOSTS", ["files.example.com"]), \
                patch.object(imagegen.vision, "fetch_image", lambda url: (b"source", "image/png")), \
                patch.object(imagegen.vision, "fit_for_model", lambda b, m: (b, m)), \
                patch.object(runpod, "run", run):
            reply = imagegen.generate_image("Alice Ally back in default outfit", 1024, 1024, requested_by="alice",
                                            image_url="https://files.example.com/a.png",
                                            people=json.dumps([dict(alice, aliases=["ally"])]))
        self.assertTrue(reply.startswith("url: "), reply)
        self.assertEqual(sent["workflow"]["4"]["inputs"]["prompt"],
                         put + "Alice back in default outfit. Keep everything else exactly the same.")

    def test_copied_look_is_left_out(self):
        # The chat model wrote alice's whole look after her name, and the prompt writer drew a woman
        # described that way and "another figure of alice" beside her.
        alice = {"name": "alice", "look": "adult woman in her mid-20s, slender willowy figure, fair skin, violet eyes, "
                 "round thin gold glasses, very long wavy lavender hair", "outfit": "a lavender hoodie"}
        bob = {"name": "bob", "look": "adult rat man, slim but muscular build, short sleek dark grey fur",
               "outfit": "a black apron"}
        for prompt, people, want in [
            ("alice figure style, adult woman in her mid-20s with slender willowy figure, fair skin, violet eyes, "
             "round thin gold glasses, very long wavy lavender hair", [alice], "alice figure style"),
            ("alice, adult woman with violet eyes, dancing in the rain at night", [alice],
             "alice, dancing in the rain at night"),
            ("bob figure style, adult bob with slim muscular build, dark grey fur", [bob], "bob figure style"),
            ("alice smiling, violet eyes sparkling under the neon lights of the city", [alice],
             "alice smiling, violet eyes sparkling under the neon lights of the city"),
            ("alice and bob at the park, slim muscular rat with dark grey fur, feeding ducks", [alice, bob],
             "alice and bob at the park, feeding ducks"),
            ("a woman with violet eyes and fair skin", [alice], "a woman with violet eyes and fair skin"),
            ("bob, slim grey rat", [{"name": "bob", "notes": ["slim grey rat"]}], "bob, slim grey rat"),
        ]:
            self.assertEqual(imagegen.look_echo_dropped(prompt, people), want, prompt)

    def test_a_named_style_without_a_lora_reaches_the_prompt_writer(self):
        # "draw me soft pastel style" came out as the prompt writer's usual clean anime illustration.
        with patch.object(imagegen, "STYLES", {"pencil": {}}):
            for request, want in [
                ("mizira just imagine me as the coolest rat and draw me soft pastel style", "soft pastel"),
                ("draw volleyball bob spiking the ball wearing in soft pastel style", "soft pastel"),
                ("draw alice in a gouache art style", "gouache"),
                ("draw it again in the same style", None),
                ("draw me in streetwear style", None),
                ("draw bob pencil style", None),
                ("draw alice https://example.com/watercolour-style.png", None),
            ]:
                got = imagegen.asked_style(request)
                self.assertEqual(got and got["description"], want, request)
        seen = []
        with patch.object(imagegen, "STYLE_FRAME", "Art style: {style}"), \
                patch.object(imagegen, "refine_prompt", REFINE_PROMPT), \
                patch.object(imagegen.chat, "endpoint", lambda name: {"url": "", "model": "", "key": ""}), \
                patch.object(imagegen, "_refine", lambda text, *a: seen.append(text) or "a rat"), \
                patch.object(imagegen, "PER_HOUR", 0), \
                patch.object(runpod, "run", lambda *a, **k: self.assertIsNone(k.get("loras")) or
                             {"images": [b"png"], "texts": ["SAFE"]}):
            reply = imagegen.generate_image("a stylish rat in soft pastel style", 1024, 1024, requested_by="bob",
                                            request="draw me soft pastel style")
        self.assertTrue(reply.startswith("url: "), reply)
        self.assertIn("Art style: soft pastel", seen[0])

    def test_role_before_a_name_dresses_them(self):
        # "volleyball bob": the chat model gave him a volleyball uniform, and he played in his apron.
        bob = {"name": "bob", "look": "adult rat man", "outfit": "a black apron"}
        for request, people, want in [
            ("mizira mizira now draw volleyball bob spiking the ball", [bob], {"bob": "volleyball"}),
            ("draw happy bob at the beach", [bob], {}),
            ("draw basketball bob", [dict(bob, variant="basketball")], {}),
            ("draw bob and alice", [bob, {"name": "alice", "look": "adult woman", "outfit": "a hoodie"}], {}),
        ]:
            self.assertEqual(imagegen.themes(request, people), want, request)
        prompt = "bob spiking the ball mid-air, wearing a sporty volleyball uniform with knee pads, sand court"
        seen = []
        with patch.object(imagegen, "PEOPLE_FRAME", "People: {people}"), \
                patch.object(imagegen, "refine_prompt", REFINE_PROMPT), \
                patch.object(imagegen.chat, "endpoint", lambda name: {"url": "", "model": "", "key": ""}), \
                patch.object(imagegen, "_refine", lambda text, *a: seen.append(text) or "bob spiking"), \
                patch.object(imagegen, "PER_HOUR", 0), \
                patch.object(runpod, "run", lambda *a, **k: {"images": [b"png"], "texts": ["SAFE"]}):
            reply = imagegen.generate_image(prompt, 1024, 1024, requested_by="bob", people=json.dumps([bob]),
                                            request="now draw volleyball bob spiking the ball")
        self.assertTrue(reply.startswith("url: "), reply)
        self.assertEqual(seen, [prompt])


if __name__ == "__main__":
    unittest.main()
