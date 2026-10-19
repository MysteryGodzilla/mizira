#!/usr/bin/env python3
# Copyright (C) 2026 BareMetal
# Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
# SPDX-License-Identifier: GPL-3.0-only

"""
view_image tool for metald.

Configure under env: in config.yml (all optional):
  VISION_API_URL   OpenAI-compatible endpoint (default: http://localhost:8080/v1)
  VISION_MODEL     model name (default: "local")
  VISION_API_KEY   bearer token, if the endpoint needs one
  VISION_ANNOUNCE  the /me shown when she starts looking (default: "is having a look at the picture");
                   worth saying something, as a model that loads for the look can take a while
"""

import sys
import os
import json

_here = os.path.dirname(os.path.abspath(__file__))
sys.path[:0] = [_here, os.path.join(_here, "lib")]
from metald_tools.vision import view_image, DEFAULT_QUESTION

ANNOUNCE = os.environ.get("VISION_ANNOUNCE", "").strip() or "is having a look at the picture"

def print_schema():
    schema = {
        "title": "view_image",
        "description": (
            "look at an image from a URL with a vision-capable model, and describe it or answer a "
            "specific question about it. slow (it can take a minute or more) and only for when "
            "someone asks you to look at a picture: never call it on your own for links in the chat."
        ),
        "type": "object",
        "properties": {
            "source": {
                "type": "string",
                "description": "http(s) URL of the image",
            },
            "question": {
                "type": "string",
                "description": "what to look for or ask about the image (default: describe it in detail)",
            },
        },
        "required": ["source"],
        "additionalProperties": False,
        # This tool needs outbound network access (to fetch the image and to
        # reach the vision API), which the default sandbox policy blocks.
        "sandbox": {"allowNetwork": True},
        # Each look can load a model: once per request, never for another bot, and said in the channel.
        "once": True,
        "nobots": True,
        "announcer": True,
    }
    print(json.dumps(schema, indent=2))

def main():
    if len(sys.argv) < 2:
        print("Usage: vision.py [--schema | --announce | --execute <json>]")
        sys.exit(1)

    option = sys.argv[1]

    if option == "--schema":
        print_schema()
        return

    if option == "--announce":
        print("action: " + ANNOUNCE)
        return

    if option == "--execute":
        if len(sys.argv) < 3:
            print("Error: Missing JSON input for execution")
            sys.exit(1)
        try:
            input_data = json.loads(sys.argv[2])
        except json.JSONDecodeError:
            print("Error: Invalid JSON input")
            sys.exit(1)

        source = input_data.get("source")
        if not source:
            print("Error: Missing required 'source' in JSON input")
            sys.exit(1)
        question = input_data.get("question") or DEFAULT_QUESTION

        print(view_image(source, question))
        return

    print("Usage: vision.py [--schema | --announce | --execute <json>]")
    sys.exit(1)

if __name__ == "__main__":
    main()
