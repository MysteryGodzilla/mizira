#!/usr/bin/env python3
# Copyright (C) 2026 BareMetal
# Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
# SPDX-License-Identifier: GPL-3.0-only

"""
generate_image tool for metald.

Configure under env: in config.yml:
  IMAGE_GEN_BACKEND    comfyui (default: the ComfyUI server at COMFYUI_URL) or runpod (a RunPod
                       serverless worker-comfyui endpoint: RUNPOD_API_KEY, RUNPOD_ENDPOINT_ID)
  COMFYUI_URL          base URL of the ComfyUI server (default: http://127.0.0.1:8188)
  IMAGE_GEN_UNET       diffusion model (default: qwen_image_2.1_int8_convrot.safetensors)
  IMAGE_GEN_CLIP       text encoder (default: qwen3vl_8b_int8_convrot.safetensors)
  IMAGE_GEN_VAE        VAE (default: qwen_image_2.1_vae_bf16.safetensors)
  IMAGE_GEN_STEPS      sampler steps (default: 25)
  IMAGE_GEN_CFG        classifier-free guidance (default: 1.0)
  IMAGE_GEN_NEGATIVE   negative prompt applied to every image (default: none)
  IMAGE_GEN_NEGATIVE_CFG  guidance used when there is a negative prompt (default: 2.5); at cfg 1
                       the sampler ignores the negative, and above it each step runs twice
  IMAGE_GEN_TIMEOUT    seconds to wait for an image (default: 540, runpod 150); keep it under the
                       bot's apitimeout
  IMAGE_GEN_EXPIRE     how long uploaded images stay up, e.g. 30d (default: the host's default)
  IMAGE_GEN_PER_HOUR, IMAGE_GEN_PER_DAY   images one person may have (default: 0, no cap)
  IMAGE_GEN_DAY_TOTAL  images everyone together may have in 24 hours (default: 0, no cap); admins'
                       images are neither capped nor counted
  IMAGE_GEN_ANNOUNCE, IMAGE_GEN_ANNOUNCE_EDIT, IMAGE_GEN_ANNOUNCE_COLD  the /me shown when a picture
                       is started, when an earlier one is edited, and with runpod when no worker is
                       running and the picture takes longer
  IMAGE_SAFETY_CHECK   vision (default: VISION_API_* looks at the image) or encoder (the image
                       model's own Qwen3-VL text encoder looks at it in the same job; runpod only)
  IMAGE_PROMPT_POLICY  what may be drawn, checked with safetyreview before anything is made
                       (IMAGE_PROMPT_REVIEW: off skips it)
  IMAGE_REFINER_URL, IMAGE_REFINER_MODEL, IMAGE_REFINER_KEY  the model that expands the prompt
                       (default: the bot's own model)
  IMAGE_EDIT_HOSTS     hosts whose pictures may be edited (image_url), comma-separated, e.g. the
                       hosting backend's own (default: none, editing off)
  IMAGE_STYLES         named styles (runpod only), one a line: name | draw, edit or edit2 | strength
                       [cfg N] | trigger | description | LoRA url (civitai.com, huggingface.co or IMAGE_STYLE_HOSTS) | sha256 [| guide].
                       draw: a new picture; edit: restyles an earlier one (the trigger is the edit, with
                       {prompt} for the request); edit2: an edit with a second picture (reference_url,
                       admins only). guide: the LoRA's own prompt advice
  IMAGE_PROMPT_STYLE   how a draw style's description reaches the prompt writer; {style}
  IMAGE_PROMPT_PEOPLE  who is in a new picture, for the prompt writer; {people}
  IMAGE_PROMPT_CHARACTERS  profile looks, added as they are after the prompt writer's prompt; {characters}
  IMAGE_REFERENCE_PROMPT  asked of an edit2 style's second picture when the request names nothing to
                       take from it; the answer fills the style's {prompt}
  hosting              see metald_tools/hosting.py (UPLOAD_BACKEND, UPLOAD_HEADERS, ...)
"""

import sys
import os
import json
import re
import time
import urllib.error
import urllib.parse

_here = os.path.dirname(os.path.abspath(__file__))
sys.path[:0] = [_here, os.path.join(_here, "lib")]
from metald_tools import toollog
from metald_tools import chat, comfyui, hosting, runpod, safetyreview, usagecap, vision
from metald_tools.hosting import HostingError, upload_file
from metald_tools.promptrefine import refine_prompt as _refine
from metald_tools.vision import check_image_safety


BACKEND = os.environ.get("IMAGE_GEN_BACKEND", "comfyui").strip().lower()
SAFETY_CHECK = os.environ.get("IMAGE_SAFETY_CHECK", "vision").strip().lower()

UNET_NAME = os.environ.get("IMAGE_GEN_UNET", "qwen_image_2.1_int8_convrot.safetensors")
CLIP_NAME = os.environ.get("IMAGE_GEN_CLIP", "qwen3vl_8b_int8_convrot.safetensors")
VAE_NAME = os.environ.get("IMAGE_GEN_VAE", "qwen_image_2.1_vae_bf16.safetensors")

# 25 steps at cfg 1 with euler/simple, per the ComfyUI template.
GEN_STEPS = int(os.environ.get("IMAGE_GEN_STEPS", "25"))
GEN_CFG = float(os.environ.get("IMAGE_GEN_CFG", "1.0"))
DEFAULT_NEGATIVE = os.environ.get("IMAGE_GEN_NEGATIVE", "").strip()
NEGATIVE_CFG = float(os.environ.get("IMAGE_GEN_NEGATIVE_CFG", "2.5"))
MAX_NEGATIVE = 500
MIN_SIDE, MAX_SIDE = 512, 1536

# render is ~35-55s locally, but with concurrent requests it can queue behind a song or video on the
# shared GPU; a RunPod job must finish inside the bot's tool timeout.
POLL_TIMEOUT = int(os.environ.get("IMAGE_GEN_TIMEOUT") or (150 if BACKEND == "runpod" else 540))
EXPIRE = os.environ.get("IMAGE_GEN_EXPIRE", "").strip()
PER_HOUR = int(os.environ.get("IMAGE_GEN_PER_HOUR") or 0)
PER_DAY = int(os.environ.get("IMAGE_GEN_PER_DAY") or 0)
DAY_TOTAL = int(os.environ.get("IMAGE_GEN_DAY_TOTAL") or 0)
ANNOUNCE = os.environ.get("IMAGE_GEN_ANNOUNCE", "").strip() or "is drawing a picture"
ANNOUNCE_COLD = (os.environ.get("IMAGE_GEN_ANNOUNCE_COLD", "").strip()
                 or "gets the drawing things out - this one takes a little longer")
ANNOUNCE_EDIT = os.environ.get("IMAGE_GEN_ANNOUNCE_EDIT", "").strip() or "is touching up the picture"

EDIT_HOSTS = [h.strip().lower() for h in os.environ.get("IMAGE_EDIT_HOSTS", "").split(",") if h.strip()]
# A RunPod request body is capped at 10 MB and the picture goes in it as base64.
MAX_SOURCE = 6 * 1024 * 1024
SOURCE_NAME = "metald_source.png"
REFERENCE_NAME = "metald_reference.png"

PROMPT_POLICY = os.environ.get("IMAGE_PROMPT_POLICY", "")
# How the people a prompt names look ("people", passed by the bot) reach the prompt writer; {people}.
PEOPLE_FRAME = os.environ.get("IMAGE_PROMPT_PEOPLE", "")
# Profile looks, added word for word after the prompt writer's prompt; {characters}.
CHARACTER_FRAME = os.environ.get("IMAGE_PROMPT_CHARACTERS", "")
# Asked of the finished edit with the encoder check; {change} is the instruction, answer YES or NO.
EDIT_CHECK = os.environ.get("IMAGE_EDIT_CHECK_PROMPT", "")
# How a drawing style reaches the prompt writer, so it doesn't describe a photo; {style}.
STYLE_FRAME = os.environ.get("IMAGE_PROMPT_STYLE", "")
# Asked of an edit2 style's second picture when the request doesn't say what to take from it ("put her
# in that outfit"): the encoder's answer fills the style's {prompt}.
REFERENCE_PROMPT = os.environ.get("IMAGE_REFERENCE_PROMPT", "")
EDITS_CHECKED = SAFETY_CHECK == "encoder" and bool(EDIT_HOSTS)
REVIEW_PROMPTS = safetyreview.enabled("IMAGE_PROMPT_REVIEW")

COLD_REPLY = ("Not ready: the image server is still starting up, which can take several minutes the first "
              "time. Tell them to ask again in a few minutes; don't retry now.")

STYLE_LINE = re.compile(r"^(?P<name>[a-z0-9]{2,20})\s*\|\s*(?P<kind>draw|edit2|edit)\s*\|\s*(?P<strength>[0-9]+(?:\.[0-9]+)?)"
                        r"(?:\s+cfg\s+(?P<cfg>[0-9]+(?:\.[0-9]+)?))?"
                        r"\s*\|(?P<trigger>[^|]*)\|(?P<description>[^|]+)\|\s*(?P<url>https://\S+)\s*\|"
                        r"\s*(?P<sha256>[0-9a-fA-F]{64})\s*(?:\|(?P<guide>[^|]*))?$")


def parse_styles(text: str) -> dict:
    """IMAGE_STYLES, one style a line: name | kind | strength [cfg N] | trigger | description | url | sha256
    [| guide]. kind: draw (a new picture), edit (restyles an earlier one) or edit2 (also takes a second
    picture, reference_url, admins only). cfg is the guidance the LoRA needs, when not the usual
    IMAGE_GEN_CFG. An edit trigger with {prompt} is filled with the request. guide is the LoRA's own
    advice: told to the prompt writer for a draw style, shown in the tool for an edit one. Blank lines
    and # comments are skipped; so is a malformed line, with a log line."""
    styles = {}
    for line in (text or "").splitlines():
        line = line.strip()
        if not line or line.startswith("#"):
            continue
        m = STYLE_LINE.match(line)
        if not m or not 0 < float(m["strength"]) <= 2 or (m["cfg"] and not 0 < float(m["cfg"]) <= 10):
            toollog.log_detail("image_gen", f"IMAGE_STYLES line skipped: {line[:80]!r}")
            continue
        styles[m["name"]] = {k: (v or "").strip() for k, v in m.groupdict().items()} | {
            "strength": float(m["strength"]), "cfg": float(m["cfg"] or 0)}
    return styles


STYLES = parse_styles(os.environ.get("IMAGE_STYLES", ""))


def style_lora(style: dict) -> dict:
    """The worker's LoRA entry; the file is named by its hash, so a changed style is a new file."""
    return {"name": f"style-{style['name']}-{style['sha256'][:12].lower()}.safetensors",
            "url": style["url"], "sha256": style["sha256"].lower()}


def check_style(name: str, editing: bool, reference: bool = False, admin: bool = False) -> tuple:
    """(the style, "") or (None, the reply saying why not); (None, "") for no style."""
    if not name:
        if reference:
            return None, "Not done: reference_url only goes with a style that takes a second picture."
        return None, ""
    style = STYLES.get(name)
    if style is None:
        return None, f"Not done: there's no style called {name!r}. Styles: {', '.join(sorted(STYLES)) or 'none'}."
    if BACKEND != "runpod":
        return None, "Error: styles need IMAGE_GEN_BACKEND runpod"
    if style["kind"] == "draw" and editing:
        # The picture already has its look; the model carried the style over from drawing it.
        toollog.log_detail("image_gen", f"style {name} dropped: it's for new pictures, this is an edit")
        return None, ""
    if style["kind"] != "draw" and not editing:
        return None, (f"Not done: {name} restyles an existing picture. Call again with that picture's "
                      "image_url (usually the latest one you posted), or offer to draw one first.")
    if style["kind"] == "edit2" and not admin:
        # The second picture is fetched from wherever its link points, which shows the bot's address.
        return None, f"Not allowed: {name} is for admins only right now. It isn't a usage limit."
    if style["kind"] == "edit2" and not reference:
        return None, f"Not done: {name} needs a second picture. Call again with its link as reference_url."
    if style["kind"] != "edit2" and reference:
        return None, f"Not done: {name} takes one picture; call again without reference_url."
    return style, ""


# Clothes a request can name. One that names none ("now wearing the outfit from the second image", as
# the model wrote it) leaves the swap nothing to go on.
GARMENTS = set("""jacket jackets coat coats blazer blazers suit suits dress dresses gown skirt skirts shirt shirts
    blouse top tops tee t-shirt tshirt hoodie hoodies sweater sweaters jumper cardigan cardigans vest waistcoat
    turtleneck pants trousers jeans shorts leggings tights stockings socks shoes shoe boots boot heels sneakers
    trainers sandals slippers hat hats cap beanie scarf gloves tie bowtie uniform kimono yukata hanbok qipao
    cheongsam apron overalls jumpsuit robe cloak cape armour armor bikini swimsuit pajamas pyjamas pjs lingerie
    corset belt bag backpack glasses sunglasses necklace earrings bracelet costume onesie kigurumi tracksuit
    raincoat parka poncho loafers stilettos pumps sweatshirt sweatpants joggers slacks tunic pinafore camisole
    bodysuit leotard windbreaker overcoat obi high-top high-tops jersey jerseys""".split())


def says_nothing(prompt: str) -> bool:
    """Whether a request names no clothes to take from the second picture."""
    return not any(w in GARMENTS for w in re.findall(r"[a-z0-9'-]+", prompt.lower()))


def style_change(style: dict, prompt: str) -> str:
    """An edit style's instruction: its trigger, with the request in place of {prompt}."""
    return style["trigger"].replace("{prompt}", prompt.strip().rstrip(".")) if style["trigger"] else prompt


def requires() -> list:
    need = ["IMAGE_SAFETY_PROMPT", "IMAGE_PROMPT_REFINER", "IMAGE_PROMPT_PEOPLE", "IMAGE_PROMPT_CHARACTERS"] + hosting.requires()
    if EDITS_CHECKED:
        need.append("IMAGE_EDIT_CHECK_PROMPT")
    if any(s["kind"] == "draw" for s in STYLES.values()):
        need.append("IMAGE_PROMPT_STYLE")
    if any(s["kind"] == "edit2" for s in STYLES.values()):
        need.append("IMAGE_REFERENCE_PROMPT")
    if REVIEW_PROMPTS:
        need += ["IMAGE_PROMPT_POLICY"] + safetyreview.requires("IMAGE_PROMPT_REVIEW")
    if BACKEND == "runpod":
        need += runpod.requires()
    return need

def print_schema():
    seconds = "15-30 seconds" if BACKEND == "runpod" else "30-60 seconds"
    schema = {
        "title": "picture",
        "description": (
            "generate an image from a text prompt and get back a public url. "
            "your prompt is automatically expanded into a more detailed, "
            "visually concrete prompt before generation - a rough idea is "
            "fine, you don't need to over-engineer it yourself. you must "
            "include that exact url in your reply so the image actually "
            f"gets posted - takes {seconds}; the channel is told you're drawing. use negative only for "
            "things the image must NOT show - usually when someone asks to "
            "leave something out or a previous render had an unwanted thing "
            "(extra arms, text, a hat). it roughly doubles render time, so "
            "leave it empty otherwise. call it only when the latest message asks for a picture, "
            "and call it every time one does: it checks limits and permissions itself, so never "
            "tell someone they are limited or not allowed without calling it for that request. when "
            "someone corrects how a person looks (\"don't forget my glasses are gold\"), redraw it and also save "
            "the correction with memory__remember. the tool itself checks what may be drawn (romance like "
            "hand holding or a kiss is fine, nudity isn't), so don't refuse a picture request yourself: call it "
            "and pass on its answer. when the picture includes people from the chat, name them rather than "
            "describing how they look: their looks are added for you; for one of their alt looks (\"her "
            "business outfit\"), write the alt's name from their profile (\"office\"). draw only what was asked: leave out "
            "what you remember about them (what they like, jokes, their bots) unless the request asks for it."
        ),
        "type": "object",
        "properties": {
            "prompt": {
                "type": "string",
                "description": "what to generate - a rough idea is fine, it gets expanded automatically",
            },
            "negative": {
                "type": "string",
                "description": "optional: comma-separated things to keep out of the image, e.g. 'text, watermark, extra fingers'",
            },
            "image_url": {
                "type": "string",
                "description": "optional: the link of an earlier picture to change instead of drawing a new one "
                               "(\"same but pink hair\"), usually the latest one you posted; prompt then names exactly "
                               "which thing changes and to what, e.g. 'make her cardigan and socks mint green', not just a colour",
            },
            "width": {"type": "integer", "description": f"image width in pixels, {MIN_SIDE}-{MAX_SIDE} (default 1024)"},
            "height": {"type": "integer", "description": f"image height in pixels, {MIN_SIDE}-{MAX_SIDE} (default 1024)"},
        } | style_property(),
        "required": ["prompt"],
        "additionalProperties": False,
        # Needs outbound network access (ComfyUI or RunPod, the hosting backend).
        "sandbox": {"allowNetwork": True},
        "requires": requires(),
        # Caps are per person, so the bot passes who asked, and another bot can't ask on someone's behalf.
        "requester": True,
        "nobots": True,
        # ...and how the people the prompt names look, from memory.
        "people": True,
        # One picture per request: it once drew a second in the same turn and posted both.
        "once": True,
        # The bot runs --announce for the /me shown while the picture is made.
        "announcer": True,
    }
    print(json.dumps(schema, indent=2))

def style_property() -> dict:
    """The schema's "style", listing the configured styles; none when there are none."""
    if not STYLES:
        return {}
    # An edit style's guide says what to put in prompt, so the model sees it; a draw style's goes to
    # the prompt writer instead.
    about = lambda s: s["description"] + (f"; {s['guide']}" if s["guide"] and s["kind"] != "draw" else "")
    listed = lambda kind: ", ".join(f"{n} ({about(s)})" for n, s in sorted(STYLES.items()) if s["kind"] == kind)
    parts = []
    if listed("draw"):
        parts.append(f"for a new picture: {listed('draw')}")
    if listed("edit"):
        parts.append(f"to restyle an earlier picture (with its image_url): {listed('edit')}")
    if listed("edit2"):
        parts.append(f"with an earlier picture (image_url) and a second one (reference_url), admins only: {listed('edit2')}")
    props = {"style": {
        "type": "string", "enum": sorted(STYLES),
        "description": "optional: a named art style, only when someone asks for that style by name or plainly "
                       "asks for that look; leave it out otherwise. " + "; ".join(parts),
    }}
    if listed("edit2"):
        props["reference_url"] = {
            "type": "string",
            "description": "optional, only with a style that takes a second picture: the link of that second "
                           "picture (e.g. the outfit to put on), exactly as someone posted it",
        }
    return props

def announcement(args: dict = None) -> str:
    """The /me for a call: an edit says so, and a cold RunPod endpoint gets the line that warns of a
    long wait. Only the call's kind is read from args; the text is always the configured one."""
    if BACKEND == "runpod" and runpod.cold():
        return "action: " + ANNOUNCE_COLD
    edit = isinstance(args, dict) and isinstance(args.get("image_url"), str) and args["image_url"].strip()
    return "action: " + (ANNOUNCE_EDIT if edit else ANNOUNCE)

def combined_negative(negative: str) -> str:
    """The configured default negative plus this request's, bounded."""
    parts = [p.strip() for p in (DEFAULT_NEGATIVE, negative or "") if p and p.strip()]
    return ", ".join(parts)[:MAX_NEGATIVE]

def side(value, default: int = 1024) -> int:
    """A width or height the model handles: within bounds, a multiple of 16."""
    try:
        n = int(value or default)
    except (TypeError, ValueError):
        n = default
    return min(MAX_SIDE, max(MIN_SIDE, n)) // 16 * 16

def build_workflow(prompt: str, width: int, height: int, negative: str = "", source: str = "",
                   change: str = "", style: dict = None, reference: str = "", describe: bool = False) -> dict:
    """source names an uploaded picture to edit; the result then keeps that picture's size. change is the
    edit's instruction, checked against the result. style adds its LoRA to the model; reference names a
    second uploaded picture, the encoder's image 2."""
    negative = combined_negative(negative)
    # cfg 1 skips the negative branch entirely, so a negative only takes effect with more guidance. A
    # style's own cfg wins: the PC-98 LoRA at cfg 1 drew retro anime without its palette or dithering.
    cfg = NEGATIVE_CFG if negative else GEN_CFG
    if style and style.get("cfg"):
        cfg = style["cfg"]
    nodes = {
        # Plain UNETLoader, not UnetLoaderGGUF: these are safetensors.
        "1": {"class_type": "UNETLoader", "inputs": {
            "unet_name": UNET_NAME, "weight_dtype": "default",
        }},
        "2": {"class_type": "CLIPLoader", "inputs": {
            "clip_name": CLIP_NAME, "type": "qwen_image", "device": "default",
        }},
        "3": {"class_type": "VAELoader", "inputs": {"vae_name": VAE_NAME}},
        "4": {"class_type": "TextEncodeQwenImage21", "inputs": {
            "clip": ["2", 0],
            "prompt": prompt,
            "negative_prompt": negative,
            "resolution": 1024,
        }},
        "5": {"class_type": "EmptyLatentImage", "inputs": {
            "width": width, "height": height, "batch_size": 1,
        }},
        "6": {"class_type": "KSampler", "inputs": {
            "seed": int(time.time() * 1000) % (2**32),
            "steps": GEN_STEPS, "cfg": cfg,
            "sampler_name": "euler", "scheduler": "simple",
            "denoise": 1.0,
            "model": ["1", 0],
            "positive": ["4", 0], "negative": ["4", 1],
            "latent_image": ["5", 0],
        }},
        "7": {"class_type": "VAEDecode", "inputs": {"samples": ["6", 0], "vae": ["3", 0]}},
        "8": {"class_type": "SaveImage", "inputs": {
            "filename_prefix": "metald_gen", "images": ["7", 0],
        }},
    }
    if source:
        # The encoder sees the picture and splices it in as reference latents; its latent output is
        # the picture's size, which keeps the edit in place.
        nodes["11"] = {"class_type": "LoadImage", "inputs": {"image": source}}
        nodes["4"]["inputs"].update({"vae": ["3", 0], "images.image_1": ["11", 0]})
        nodes["6"]["inputs"]["latent_image"] = ["4", 2]
        del nodes["5"]
        if reference:
            nodes["14"] = {"class_type": "LoadImage", "inputs": {"image": reference}}
            nodes["4"]["inputs"]["images.image_2"] = ["14", 0]
        if reference and describe:
            # The encoder says what the second picture shows, and that fills the prompt's {prompt}
            # before it is encoded, in the same job.
            nodes["15"] = {"class_type": "TextGenerate", "inputs": {
                "clip": ["2", 0], "prompt": REFERENCE_PROMPT, "image": ["14", 0],
                "max_length": 60, "sampling_mode": "off",
            }}
            nodes["16"] = {"class_type": "PreviewAny", "inputs": {"source": ["15", 0]}}
            nodes["17"] = {"class_type": "StringReplace", "inputs": {
                "string": prompt, "find": "{prompt}", "replace": ["15", 0],
            }}
            nodes["4"]["inputs"]["prompt"] = ["17", 0]
    if style:
        nodes["20"] = {"class_type": "LoraLoaderModelOnly", "inputs": {
            "model": ["1", 0], "lora_name": style_lora(style)["name"], "strength_model": style["strength"],
        }}
        nodes["6"]["inputs"]["model"] = ["20", 0]
    if SAFETY_CHECK == "encoder":
        # The Qwen3-VL text encoder already loaded for the prompt looks at the finished image.
        nodes["9"] = {"class_type": "TextGenerate", "inputs": {
            "clip": ["2", 0], "prompt": vision.SAFETY_PROMPT, "image": ["7", 0],
            "max_length": 40, "sampling_mode": "off",
        }}
        nodes["10"] = {"class_type": "PreviewAny", "inputs": {"source": ["9", 0]}}
        if source and change and EDIT_CHECK:
            # An edit about 1 in 5 times came back without the change: the encoder says whether it shows.
            nodes["12"] = {"class_type": "TextGenerate", "inputs": {
                "clip": ["2", 0], "prompt": EDIT_CHECK.replace("{change}", change), "image": ["7", 0],
                "max_length": 8, "sampling_mode": "off",
            }}
            nodes["13"] = {"class_type": "PreviewAny", "inputs": {"source": ["12", 0]}}
    return {"prompt": nodes}

def upload_image(image_bytes: bytes) -> str:
    return upload_file(image_bytes, f"metald_{int(time.time() * 1000)}.png", "image/png", EXPIRE)

def parse_people(people: str) -> list:
    """The bot's "people": [{name, aliases, drawn_as, look, outfit, variant, notes}]; [] when there are none."""
    try:
        found = json.loads(people) if people.strip() else []
    except ValueError:
        toollog.log_detail("image_gen", f"people unreadable: {people[:80]!r}")
        return []
    if not isinstance(found, list):
        return []
    return [p for p in found if isinstance(p, dict) and isinstance(p.get("name"), str)]


def people_lines(people: list) -> str:
    """The prompt writer's view: someone with a set look is named, with their outfit; anyone else is
    described from their look memories."""
    lines = []
    for p in people:
        sex = p.get("drawn_as") or ""
        if p.get("look") and not p.get("outfit"):
            # Dressed as the request says, the line only told the prompt writer of "the second person ...
            # wearing the specified outfit from their entry".
            continue
        if p.get("look"):
            # Without "drawn as", "MysteryGodzilla" in a dinosaur onesie became "he ... massive reptilian form".
            drawn = f"; drawn as {sex}" if sex else ""
            lines.append(f"{p['name']} - outfit: {p['outfit']}{drawn}")
        else:
            about = [f"drawn as {sex}" if sex else ""] + list(p.get("notes") or [])
            lines.append(f"{p['name']}: {'; '.join(x for x in about if x)}")
    return "\n".join(lines)


def bind_looks(prompt: str, people: list) -> tuple:
    """(prompt with each set look right after the first mention of its person's name, the people with a
    look it doesn't name). As a separate list the look drew a second person beside the one described."""
    unbound = []
    for p in people:
        if not p.get("look"):
            continue
        m = re.search(r"(?i)(?<![\w-])" + re.escape(p["name"]) + r"(?![\w-])", prompt)
        if m:
            prompt = f"{prompt[:m.end()]} ({p['look']}){prompt[m.end():]}"
        else:
            unbound.append(p)
    if len(unbound) == 1:
        # The prompt writer dropped the name ("a young adult woman with long blue hair"), and listed
        # apart, the look was drawn as a second woman: it goes on the first one of theirs instead.
        p = unbound[0]
        m = re.search(r"(?i)\b(?:a|an|one|the)\s+(?:[\w-]+\s+){0,4}?" + PERSON_WORDS[p.get("drawn_as") or ""] + r"\b", prompt)
        if m:
            toollog.log_detail("image_gen", f"look of {p['name']} put on {m.group(0)!r}")
            prompt = f"{prompt[:m.end()]} named {p['name']} ({p['look']}){prompt[m.end():]}"
            unbound = []
    return prompt, unbound


# Words for one person, by how they're drawn, that an unnamed look can go on.
PERSON_WORDS = {"female": "(?:woman|girl|lady|heroine)", "male": "(?:man|boy|guy|hero)",
                "": "(?:person|figure|character)"}


def one_name(prompt: str, people: list) -> str:
    """The prompt with each person's other names as their name, said once: in "alice Ally sitting" the
    look went in after alice and Ally was drawn as a second woman."""
    for p in people:
        name = p["name"]
        for alias in sorted((a for a in p.get("aliases") or [] if isinstance(a, str) and a), key=len, reverse=True):
            prompt = re.sub(r"(?i)(?<![\w-])" + re.escape(alias) + r"(?![\w-])", lambda _: name, prompt)
        n = re.escape(name)
        prompt = re.sub(rf"(?i)(?<![\w-])({n})(?:\s*\(\s*{n}\s*\)|\s+{n})+(?![\w'’-])", lambda m: m.group(1), prompt)
    return prompt


def named_first(prompt: str, people: list) -> str:
    """The prompt with "adult woman mid-20s (alice)" as "alice (adult woman mid-20s)": with the name
    tucked in brackets after a description, the prompt writer drew the woman and "another person named
    alice" beside her."""
    for p in people:
        n = re.escape(p["name"])
        prompt = re.sub(rf"(?i)(?<![\w'-])(?:(?:an?|the)\s+)?((?:(?!(?:of|in|on|at|with|for|and|the|an?|to|from|by|as)\b)[\w'-]+\s+){{1,4}})"
                        rf"\(\s*{n}\s*\)", lambda m: f"{p['name']} ({m.group(1).strip()})", prompt)
    return prompt


def unasked_clothes_dropped(prompt: str, request: str, people: list) -> str:
    """The prompt without its parts that dress people in clothes the asker never named, when someone in
    it has a set outfit: the chat model made up "a modern tech outfit: turtleneck, cargo pants", and the
    prompt writer drew a second woman to wear the set one. A part naming someone stays."""
    if not request or not any(p.get("look") and p.get("outfit") for p in people):
        return prompt
    asked = set(re.findall(r"[a-z0-9'-]+", request.lower()))
    names = [p["name"].lower() for p in people]
    kept, dropped = [], []
    for part in re.split(r"(?<=[,.;:])\s+", prompt):
        words = set(re.findall(r"[a-z0-9'-]+", part.lower()))
        if any(not said(w, asked) for w in words & WORN) and not any(n in words for n in names):
            dropped.append(part)
        else:
            kept.append(part)
    if not dropped or not kept:
        return prompt
    toollog.log_detail("image_gen", f"clothes nobody asked for left out: {' '.join(dropped)[:200]!r}")
    return " ".join(kept).rstrip(",;: ")


def said(word: str, asked: set) -> bool:
    """Whether the request has word, allowing two swapped letters ("jeresy" for jersey)."""
    return word in asked or any(len(a) == len(word) and any(a[:i] + a[i + 1] + a[i] + a[i + 2:] == word
                                                            for i in range(len(a) - 1)) for a in asked)


def outfit_from_request(people: list, request: str) -> list:
    """The people, without their set outfit when the request names a whole outfit ("me in a ravens
    jersey") and no variant: given both, the prompt writer kept her thigh-high socks under the jersey,
    and once drew "two people on a plush couch"."""
    asked = set(re.findall(r"[a-z0-9'-]+", request.lower()))
    if not any(said(w, asked) for w in OUTFIT_WORDS):
        return people
    out = []
    for p in people:
        if p.get("look") and p.get("outfit") and not p.get("variant"):
            toollog.log_detail("image_gen", f"{p['name']}: the request's clothes, not the set outfit")
            p = dict(p, outfit="")
        out.append(p)
    return out


# Words that dress someone; "top", "cap" and "glasses" are left out as they're scenery as often.
WORN = (GARMENTS - {"top", "tops", "cap", "tie", "bag", "backpack", "belt", "glasses", "sunglasses", "necklace",
                    "earrings", "bracelet"}) | {"outfit", "outfits", "clothes", "clothing", "attire", "dressed"}
# Words for a whole outfit; a hat, socks or shoes go with the set one.
OUTFIT_WORDS = GARMENTS - {"hat", "hats", "cap", "beanie", "scarf", "gloves", "tie", "bowtie", "belt", "bag", "backpack",
                           "glasses", "sunglasses", "necklace", "earrings", "bracelet", "socks", "shoes", "shoe", "boots",
                           "boot", "heels", "sneakers", "trainers", "sandals", "slippers", "loafers", "stilettos", "pumps",
                           "high-top", "high-tops", "tights", "stockings", "apron", "cape", "cloak", "top", "tops"}


# What an edit calls someone's clothes without saying what they are ("back in her default outfit").
CLOTHES_WORDS = {"outfit", "outfits", "clothes", "clothing", "attire", "dressed", "wardrobe"}


def dressed(change: str, people: list) -> str:
    """An edit putting someone in their set outfit, with the outfit spelled out: the edit model was told
    "back in default outfit" and had nothing to go on. Only for the one person with a set look, named
    in the change or the only one in the picture, and only when the change names no clothes itself."""
    looks = [p for p in people if p.get("look") and p.get("outfit")]
    words = set(re.findall(r"[a-z0-9'-]+", change.lower()))
    if len(looks) != 1 or not says_nothing(change):
        return change
    p = looks[0]
    if not (words & CLOTHES_WORDS or p.get("variant") in words):
        return change
    if len(people) > 1 and p["name"].lower() not in words:
        return change
    their = {"female": "her", "male": "his"}.get(p.get("drawn_as"), "their")
    return f"Change {their} outfit to {p['outfit']}. {change}"


def characters_text(people: list) -> str:
    """The set looks, added to the final prompt as they are."""
    return "; ".join(f"{p['name']}: {p['look']}" for p in people if p.get("look"))


def refine_prompt(user_prompt: str, people: str = "", style: dict = None) -> str:
    """The detailed prompt; people (passed by the bot) says who is in it and how they look, and a
    drawing style what it's drawn in."""
    lines = people_lines(parse_people(people))
    if lines and PEOPLE_FRAME.strip():
        user_prompt += "\n\n" + PEOPLE_FRAME.strip().replace("{people}", lines)
    if style and STYLE_FRAME.strip():
        about = style["description"] + (f". {style['guide']}" if style["guide"] else "")
        user_prompt += "\n\n" + STYLE_FRAME.strip().replace("{style}", about)
    ep = chat.endpoint("IMAGE_REFINER")
    return _refine(user_prompt, ep["url"], ep["model"], ep["key"])

def review_prompt(prompt: str, edit: bool = False) -> str:
    """Why the request may not be drawn, as the tool's reply, or "" when it may."""
    if not REVIEW_PROMPTS:
        return ""
    # A bare "make her hair pink" reads as missing context unless the reviewer knows it's a change.
    content = f"Change to an existing picture: {prompt}" if edit else prompt
    allowed, reason = safetyreview.review(PROMPT_POLICY, content, "IMAGE REQUEST",
                                          logger=lambda m: toollog.log_detail("image_gen", f"prompt review: {m}"))
    if allowed:
        return ""
    if reason.lower().startswith("character"):
        who = reason[len("character"):].strip(" :-") or "that character"
        return (f"Not drawn: {who} is someone else's copyrighted character. Offer to draw an original "
                "character in a similar style instead; if they agree, call again describing the original "
                "without naming the character.")
    return f"Error: refused - that request can't be drawn ({reason})."

def edit_prompt(change: str) -> str:
    """The change, plus keeping the rest: "mint green instead of purple" also recoloured lilac hair."""
    change = change.strip().rstrip(".")
    return change + ". Keep everything else exactly the same."

def fetch_source(url: str) -> tuple:
    """(picture bytes, "") for a picture that may be edited, or (None, the reply saying why not)."""
    host = (urllib.parse.urlparse(url).hostname or "").lower()
    if host not in EDIT_HOSTS:
        allowed = ", ".join(EDIT_HOSTS) or "nowhere: editing is off"
        return None, (f"Not edited: only pictures on {allowed} can be changed. Offer to draw a new one "
                      "from a description instead.")
    try:
        data, mime = vision.fetch_image(url)
    except (urllib.error.URLError, ValueError, OSError) as e:
        toollog.log_detail("image_gen", f"edit source unreadable: {e}")
        return None, "Error: couldn't fetch that picture to edit; check the link."
    if not mime.startswith("image/"):
        return None, "Error: that link isn't a picture."
    data, _ = vision.fit_for_model(data, mime)
    if len(data) > MAX_SOURCE:
        return None, "Error: that picture is too large to edit."
    return data, ""

def fetch_reference(url: str) -> tuple:
    """(picture bytes, "") for an edit2 style's second picture, from any public link (admins only, so
    checked by urlguard rather than IMAGE_EDIT_HOSTS), or (None, the reply saying why not)."""
    try:
        data, mime = vision.fetch_image(url)
    except (urllib.error.URLError, ValueError, OSError) as e:
        toollog.log_detail("image_gen", f"reference unreadable: {e}")
        return None, "Error: couldn't fetch the second picture; check its link."
    if not mime.startswith("image/"):
        return None, "Error: the second link isn't a picture."
    data, _ = vision.fit_for_model(data, mime)
    if len(data) > MAX_SOURCE:
        return None, "Error: the second picture is too large."
    return data, ""

def render(workflow: dict, source: bytes = None, style: dict = None, reference: bytes = None) -> tuple:
    """(image bytes, safety verdict or None, edit check or ""). The verdict and the check come with the
    image when the encoder looks at it; the texts arrive in node order, safety first."""
    log = lambda m: toollog.log_detail("image_gen", m)
    if BACKEND == "runpod":
        out = runpod.run(workflow["prompt"], POLL_TIMEOUT, log=log,
                         inputs=({SOURCE_NAME: source} | ({REFERENCE_NAME: reference} if reference else {}))
                         if source else None,
                         loras=[style_lora(style)] if style else None)
        texts = out["texts"]
        verdict = texts[0] if texts else ""
        check = texts[1] if "12" in workflow["prompt"] and len(texts) > 1 else ""
        if "16" in workflow["prompt"] and len(texts) > 1:
            log(f"second picture described as: {texts[1][:200]!r}")
        return out["images"][0], (verdict if SAFETY_CHECK == "encoder" else None), check
    if source:
        workflow["prompt"]["11"]["inputs"]["image"] = comfyui.upload(source, SOURCE_NAME, log=log)
    return comfyui.run(workflow, "images", POLL_TIMEOUT, log=log), None, ""

def edit_missed(check: str) -> bool:
    """Whether the encoder said the edit doesn't show; no answer isn't a miss."""
    return check.strip().upper().startswith("NO")

def generate_image(prompt: str, width: int, height: int, negative: str = "", requested_by: str = "",
                   image_url: str = "", people: str = "", admin: bool = False, style: str = "",
                   reference_url: str = "", request: str = "") -> str:
    if SAFETY_CHECK == "encoder" and BACKEND != "runpod":
        return "Error: IMAGE_SAFETY_CHECK encoder needs IMAGE_GEN_BACKEND runpod"
    chosen, why_not = check_style(style, bool(image_url), bool(reference_url), admin)
    if why_not:
        return why_not
    # Admins are neither capped nor counted, so the shared day total stays everyone else's.
    if admin:
        return draw(prompt, width, height, negative, image_url, people, chosen, reference_url, request)
    over = usagecap.take("imagegen", requested_by, PER_HOUR, PER_DAY, DAY_TOTAL)
    if over:
        return over
    reply = draw(prompt, width, height, negative, image_url, people, chosen, reference_url, request)
    if not reply.startswith("url: "):
        # Only a posted picture counts against the caps.
        usagecap.give_back("imagegen", requested_by)
    return reply

def draw(prompt: str, width: int, height: int, negative: str, image_url: str = "", people: str = "",
         style: dict = None, reference_url: str = "", request: str = "") -> str:
    refused = review_prompt(prompt, edit=bool(image_url))
    if refused:
        return refused
    source = None
    if image_url:
        source, why_not = fetch_source(image_url)
        if source is None:
            return why_not
    reference = None
    if reference_url:
        reference, why_not = fetch_reference(reference_url)
        if reference is None:
            return why_not
    named = parse_people(people)
    prompt = named_first(one_name(prompt, named), named)
    if not source:
        prompt = unasked_clothes_dropped(prompt, request, named)
        named = outfit_from_request(named, request)
    if named:
        toollog.log_detail("image_gen", f"asked for: {prompt[:300]!r}")
        toollog.log_detail("image_gen", "people: " + ", ".join(
            p["name"] + (f" (look{': ' + p['variant'] if p.get('variant') else ''})" if p.get("look")
                         else " (memories)" if p.get("notes") else "") for p in named))
    if style:
        toollog.log_detail("image_gen", f"style: {style['name']}")
    describe = False
    if source and style and "{prompt}" in style["trigger"]:
        # The LoRA's own wording with the request in it (a camera angle, an outfit), as it was trained;
        # the encoder can't judge those, so no edit check.
        change, final_prompt = "", style_change(style, prompt)
        if reference and says_nothing(prompt) and REFERENCE_PROMPT:
            # "Put her in that outfit" left the swap nothing to go on; the encoder describes it instead.
            describe, final_prompt = True, style["trigger"]
    elif source:
        # An edit style's LoRA was trained on its own instruction, so that is the whole edit. An edit is
        # a short instruction about the picture; expanding it would redraw the whole scene.
        change = style["trigger"] if style and style["trigger"] else dressed(prompt, named)
        final_prompt = edit_prompt(change)
        toollog.log_detail("image_gen", f"edit: {final_prompt[:600]!r}")
    else:
        change = ""
        final_prompt = refine_prompt(prompt, json.dumps(named), style)
        if style and style["trigger"]:
            final_prompt = f"{style['trigger']}, {final_prompt}"
        final_prompt, unbound = bind_looks(final_prompt, named)
        if characters_text(unbound) and CHARACTER_FRAME.strip():
            final_prompt += "\n\n" + CHARACTER_FRAME.strip().replace("{characters}", characters_text(unbound))
        toollog.log_detail("image_gen", f"prompt: {final_prompt[:600]!r}")
    try:
        workflow = lambda: build_workflow(final_prompt, width, height, negative, SOURCE_NAME if source else "",
                                          change, style, REFERENCE_NAME if reference else "", describe)
        image_bytes, verdict, check = render(workflow(), source, style, reference)
        if source and edit_missed(check):
            # One more try, with a new seed; the second result stands either way.
            toollog.log_detail("image_gen", f"edit missed ({check.strip()[:40]!r}), trying again")
            image_bytes, verdict, check = render(workflow(), source, style, reference)
    except comfyui.ComfyCancelled:
        return comfyui.CANCELLED_REPLY
    except comfyui.ComfyError as e:
        if str(e) == "backend unreachable":
            return "Error: image backend is unavailable right now"
        if str(e) == "generation timed out" and BACKEND == "runpod":
            return COLD_REPLY
        return "Error: image generation failed"

    if verdict is None:
        is_safe, verdict = check_image_safety(image_bytes)
    else:
        is_safe = vision.says_safe(verdict)
        toollog.log_detail("image_gen", f"encoder safety verdict: {verdict[:200]!r}")
    if not is_safe:
        return (
            f"Error: generated image did not pass the safety check and was "
            f"not posted ({(verdict or 'no verdict')[:200]}). try a different prompt."
        )

    try:
        url = upload_image(image_bytes)
    except (HostingError, urllib.error.URLError, urllib.error.HTTPError, OSError) as e:
        # HostingError embeds the host's URL and raw response body.
        toollog.log_detail("image_gen", f"upload failed: {e}")
        return "Error: image generated but the upload failed"

    return f"url: {url}"

def main():
    if len(sys.argv) < 2:
        print("Usage: image_gen.py [--schema | --announce | --execute <json>]")
        sys.exit(1)

    option = sys.argv[1]

    if option == "--schema":
        print_schema()
        return

    if option == "--announce":
        try:
            args = json.loads(sys.argv[2]) if len(sys.argv) > 2 else {}
        except json.JSONDecodeError:
            args = {}
        print(announcement(args))
        return

    if option == "--execute":
        if BACKEND != "runpod" and not comfyui.online():
            print(comfyui.OFFLINE_REPLY)
            return
        if len(sys.argv) < 3:
            print("Error: Missing JSON input for execution")
            sys.exit(1)
        try:
            input_data = json.loads(sys.argv[2])
        except json.JSONDecodeError:
            print("Error: Invalid JSON input")
            sys.exit(1)

        prompt = input_data.get("prompt")
        if not prompt:
            print("Error: Missing required 'prompt' in JSON input")
            sys.exit(1)
        # 1024 is Qwen-Image 2.1's native size.
        width, height = side(input_data.get("width")), side(input_data.get("height"))

        negative = input_data.get("negative") or ""
        if not isinstance(negative, str):
            negative = ""
        print(generate_image(prompt, width, height, negative, str(input_data.get("requested_by") or ""),
                             str(input_data.get("image_url") or "").strip(), str(input_data.get("people") or ""),
                             input_data.get("requested_by_admin") is True,
                             str(input_data.get("style") or "").strip().lower(),
                             str(input_data.get("reference_url") or "").strip(),
                             str(input_data.get("request") or "")))
        return

    print("Usage: image_gen.py [--schema | --announce | --execute <json>]")
    sys.exit(1)

if __name__ == "__main__":
    main()
