#!/usr/bin/env python3
# Copyright (C) 2026 MysteryGodzilla
# Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
# SPDX-License-Identifier: GPL-3.0-only
import os
import sys
import json
import threading
import subprocess
import unittest
from http.server import BaseHTTPRequestHandler, HTTPServer

_plugins = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
PASTE = os.path.join(_plugins, "paste.py")


class Host(BaseHTTPRequestHandler):
    seen = []

    def do_POST(self):
        body = self.rfile.read(int(self.headers.get("Content-Length", 0)))
        Host.seen.append({"headers": {k.lower(): v for k, v in self.headers.items()}, "body": body})
        self.send_response(200)
        self.end_headers()
        self.wfile.write(b"https://files.example.com/abc.py\n")

    def log_message(self, *a):
        pass


class PasteHostingTest(unittest.TestCase):
    @classmethod
    def setUpClass(cls):
        cls.server = HTTPServer(("127.0.0.1", 0), Host)
        threading.Thread(target=cls.server.serve_forever, daemon=True).start()
        cls.env = {k: v for k, v in os.environ.items()
                   if not k.startswith(("UPLOAD_", "RUSTYPASTE_", "GIST_", "PASTE_", "ZIPLINE_"))}
        cls.env.update({"PASTE_BACKEND": "hosting", "UPLOAD_BACKEND": "rustypaste",
                        "RUSTYPASTE_URL": "http://127.0.0.1:%d/" % cls.server.server_port,
                        "RUSTYPASTE_TOKEN": "tok", "PASTE_SAFETY_POLICY": "allow code",
                        "PASTE_REVIEW": "off", "UPLOAD_RETRIES": "1", "METALD_TOOL_LOG": os.devnull})

    @classmethod
    def tearDownClass(cls):
        cls.server.shutdown()

    def run_paste(self, *args):
        out = subprocess.run([sys.executable, PASTE, *args], env=self.env, capture_output=True, text=True,
                             timeout=60, stdin=subprocess.DEVNULL)
        return out.stdout

    def test_schema_requires_the_host_not_opengist(self):
        req = json.loads(self.run_paste("--schema"))["requires"]
        self.assertIn("RUSTYPASTE_TOKEN", req)
        self.assertNotIn("GIST_TOKEN", req)

    def test_paste_goes_to_the_host_with_its_expiry(self):
        Host.seen = []
        out = self.run_paste("--execute", json.dumps({"content": "print('hi')\nprint('bye')",
                                                      "language": "python", "title": "demo", "expire": "1hour"}))
        self.assertIn("url: https://files.example.com/abc.py", out)
        self.assertIn("lines: 2", out)
        sent = Host.seen[-1]
        self.assertEqual(sent["headers"]["expire"], "1h")
        self.assertEqual(sent["headers"]["authorization"], "tok")
        self.assertIn(b'filename="demo.py"', sent["body"])
        self.assertIn(b"print('hi')\nprint('bye')", sent["body"])

    def test_longer_expiry_is_clamped_to_a_day(self):
        Host.seen = []
        self.run_paste("--execute", json.dumps({"content": "x", "expire": "never"}))
        self.assertEqual(Host.seen[-1]["headers"]["expire"], "1d")


if __name__ == "__main__":
    unittest.main()
