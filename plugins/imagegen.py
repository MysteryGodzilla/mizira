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
  IMAGE_GEN_DAY_TOTAL  images everyone together may have in 24 hours (default: 0, no cap)
  IMAGE_GEN_ANNOUNCE, IMAGE_GEN_ANNOUNCE_COLD  the /me shown when a picture is started, and with
                       runpod when no worker is up yet and the first picture will take minutes
  IMAGE_SAFETY_CHECK   vision (default: VISION_API_* looks at the image) or encoder (the image
                       model's own Qwen3-VL text encoder looks at it in the same job; runpod only)
  IMAGE_PROMPT_POLICY  what may be drawn, checked with safetyreview before anything is made
                       (IMAGE_PROMPT_REVIEW: off skips it)
  IMAGE_REFINER_URL, IMAGE_REFINER_MODEL, IMAGE_REFINER_KEY  the model that expands the prompt
                       (default: the bot's own model)
  hosting              see metald_tools/hosting.py (UPLOAD_BACKEND, UPLOAD_HEADERS, ...)
"""

import sys
import os
import json
import time
import urllib.error

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
                 or "gets the drawing things out - the first picture takes a few minutes")

PROMPT_POLICY = os.environ.get("IMAGE_PROMPT_POLICY", "")
REVIEW_PROMPTS = safetyreview.enabled("IMAGE_PROMPT_REVIEW")

COLD_REPLY = ("Not ready: the image server is still starting up, which can take several minutes the first "
              "time. Tell them to ask again in a few minutes; don't retry now.")


def requires() -> list:
    need = ["IMAGE_SAFETY_PROMPT", "IMAGE_PROMPT_REFINER"] + hosting.requires()
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
            "leave it empty otherwise."
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
            "width": {"type": "integer", "description": f"image width in pixels, {MIN_SIDE}-{MAX_SIDE} (default 1024)"},
            "height": {"type": "integer", "description": f"image height in pixels, {MIN_SIDE}-{MAX_SIDE} (default 1024)"},
        },
        "required": ["prompt"],
        "additionalProperties": False,
        # Needs outbound network access (ComfyUI or RunPod, the hosting backend).
        "sandbox": {"allowNetwork": True},
        "requires": requires(),
        # Caps are per person, so the bot passes who asked.
        "requester": True,
        # The bot runs --announce for the /me shown while the picture is made.
        "announcer": True,
    }
    print(json.dumps(schema, indent=2))

def announcement() -> str:
    """The /me for a new picture: a cold RunPod endpoint gets the line that warns of a long wait."""
    cold = BACKEND == "runpod" and runpod.cold()
    return "action: " + (ANNOUNCE_COLD if cold else ANNOUNCE)

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

def build_workflow(prompt: str, width: int, height: int, negative: str = "") -> dict:
    negative = combined_negative(negative)
    # cfg 1 skips the negative branch entirely, so a negative only takes effect with more guidance.
    cfg = NEGATIVE_CFG if negative else GEN_CFG
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
    if SAFETY_CHECK == "encoder":
        # The Qwen3-VL text encoder already loaded for the prompt looks at the finished image.
        nodes["9"] = {"class_type": "TextGenerate", "inputs": {
            "clip": ["2", 0], "prompt": vision.SAFETY_PROMPT, "image": ["7", 0],
            "max_length": 40, "sampling_mode": "off",
        }}
        nodes["10"] = {"class_type": "PreviewAny", "inputs": {"source": ["9", 0]}}
    return {"prompt": nodes}

def upload_image(image_bytes: bytes) -> str:
    return upload_file(image_bytes, f"metald_{int(time.time() * 1000)}.png", "image/png", EXPIRE)

def refine_prompt(user_prompt: str) -> str:
    ep = chat.endpoint("IMAGE_REFINER")
    return _refine(user_prompt, ep["url"], ep["model"], ep["key"])

def review_prompt(prompt: str) -> str:
    """Why the request may not be drawn, as the tool's reply, or "" when it may."""
    if not REVIEW_PROMPTS:
        return ""
    allowed, reason = safetyreview.review(PROMPT_POLICY, prompt, "IMAGE REQUEST",
                                          logger=lambda m: toollog.log_detail("image_gen", f"prompt review: {m}"))
    if allowed:
        return ""
    if reason.lower().startswith("character"):
        who = reason[len("character"):].strip(" :-") or "that character"
        return (f"Not drawn: {who} is someone else's copyrighted character. Offer to draw an original "
                "character in a similar style instead; if they agree, call again describing the original "
                "without naming the character.")
    return f"Error: refused - that request can't be drawn ({reason})."

def render(workflow: dict) -> tuple:
    """(image bytes, safety verdict or None). The verdict comes with the image when the encoder checks it."""
    log = lambda m: toollog.log_detail("image_gen", m)
    if BACKEND == "runpod":
        out = runpod.run(workflow["prompt"], POLL_TIMEOUT, log=log)
        verdict = out["texts"][0] if out["texts"] else ""
        return out["images"][0], (verdict if SAFETY_CHECK == "encoder" else None)
    return comfyui.run(workflow, "images", POLL_TIMEOUT, log=log), None

def generate_image(prompt: str, width: int, height: int, negative: str = "", requested_by: str = "") -> str:
    if SAFETY_CHECK == "encoder" and BACKEND != "runpod":
        return "Error: IMAGE_SAFETY_CHECK encoder needs IMAGE_GEN_BACKEND runpod"
    over = usagecap.take("imagegen", requested_by, PER_HOUR, PER_DAY, DAY_TOTAL)
    if over:
        return over
    reply = draw(prompt, width, height, negative)
    if not reply.startswith("url: "):
        # Only a posted picture counts against the caps.
        usagecap.give_back("imagegen", requested_by)
    return reply

def draw(prompt: str, width: int, height: int, negative: str) -> str:
    refused = review_prompt(prompt)
    if refused:
        return refused
    refined_prompt = refine_prompt(prompt)
    try:
        image_bytes, verdict = render(build_workflow(refined_prompt, width, height, negative))
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
        print(announcement())
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
        print(generate_image(prompt, width, height, negative, str(input_data.get("requested_by") or "")))
        return

    print("Usage: image_gen.py [--schema | --announce | --execute <json>]")
    sys.exit(1)

if __name__ == "__main__":
    main()
