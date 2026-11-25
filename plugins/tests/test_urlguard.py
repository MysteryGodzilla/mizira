#!/usr/bin/env python3
# Copyright (C) 2026 BareMetal
# Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
# SPDX-License-Identifier: GPL-3.0-only
import ipaddress
import os
import socket
import sys
import unittest
import urllib.error
import urllib.request
from unittest.mock import patch

os.environ.setdefault("METALD_TOOL_LOG", os.devnull)
_plugins = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
sys.path[:0] = [_plugins, os.path.join(_plugins, "lib"), os.path.dirname(os.path.abspath(__file__))]
from fakeserver import FakeServer
from metald_tools import urlguard


def resolving(*answers):
    """A getaddrinfo that gives each answer in turn (the last one from then on), for any name."""
    left = list(answers)

    def fake(host, port, *a, **k):
        addr = left.pop(0) if len(left) > 1 else left[0]
        return [(socket.AF_INET, socket.SOCK_STREAM, 6, "", (addr, port or 0))]
    return fake


class UrlGuardTest(unittest.TestCase):
    def setUp(self):
        self.srv = FakeServer({"/x": lambda p, b: (200, b"inside"),
                               "/moved": lambda p, b: (302, b"", {"Location": self.srv.url + "/x"})})
        self.addCleanup(self.srv.close)
        self.port = self.srv.httpd.server_port

    def test_only_global_addresses_are_public(self):
        for addr, want in [("8.8.8.8", True), ("2606:4700::1111", True), ("100.64.0.1", False),
                           ("100.127.255.254", False), ("10.0.0.1", False), ("172.16.0.1", False),
                           ("127.0.0.1", False), ("169.254.169.254", False), ("0.0.0.0", False),
                           ("224.0.0.1", False), ("::1", False), ("fe80::1", False), ("fd00::1", False),
                           ("::ffff:127.0.0.1", False), ("192.0.2.1", False)]:
            self.assertEqual(urlguard.public(ipaddress.ip_address(addr)), want, addr)

    def test_a_tailnet_address_is_refused(self):
        with patch.object(urlguard.socket, "getaddrinfo", resolving("100.64.0.1")):
            self.assertEqual(urlguard.safe_url("http://files.example.com/a.png"), (None, "that address is not public"))

    def test_a_name_that_changes_after_the_check_never_reaches_it(self):
        # Public for the check, loopback for the fetch: the connection is refused, and nothing arrives.
        url = f"http://files.example.com:{self.port}/x"
        with patch.object(urlguard.socket, "getaddrinfo", resolving("8.8.8.8", "127.0.0.1")):
            parsed, err = urlguard.safe_url(url)
            self.assertIsNone(err)
            with self.assertRaises(urllib.error.URLError) as caught:
                urlguard.opener().open(url, timeout=5)
        self.assertIn("not public", str(caught.exception))
        self.assertEqual(self.srv.seen, [])

    def test_the_fetch_goes_to_the_checked_address_with_its_own_host(self):
        with patch.object(urlguard, "public", lambda addr: True), \
                patch.object(urlguard.socket, "getaddrinfo", resolving("127.0.0.1")), \
                patch.dict(os.environ, {"http_proxy": "http://127.0.0.1:1", "HTTP_PROXY": "http://127.0.0.1:1"}):
            with urlguard.opener().open(f"http://files.example.com:{self.port}/x", timeout=5) as resp:
                self.assertEqual(resp.read(), b"inside")
        self.assertEqual(self.srv.seen[0]["headers"]["host"], f"files.example.com:{self.port}")

    def test_redirects_are_not_followed(self):
        with patch.object(urlguard, "public", lambda addr: True):
            with self.assertRaises(urllib.error.HTTPError) as caught:
                urlguard.opener().open(self.srv.url + "/moved", timeout=5)
        self.assertEqual(caught.exception.code, 302)
        self.assertEqual([r["path"] for r in self.srv.seen], ["/moved"])

    def test_https_keeps_the_hostname_for_the_certificate(self):
        conn = urlguard._HTTPS("files.example.com", 443)
        self.assertIs(conn._create_connection, urlguard.connect)
        self.assertEqual(conn.host, "files.example.com")


if __name__ == "__main__":
    unittest.main()
