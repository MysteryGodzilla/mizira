#!/usr/bin/env python3
# Copyright (C) 2026 BareMetal
# Part of metald, a fork of soulshack (github.com/pkdindustries/soulshack)
# SPDX-License-Identifier: GPL-3.0-only

"""
Shared URL safety check for tools that fetch something a user named.

safe_url() gives a reason to refuse up front; opener() is what keeps the fetch safe: it checks the
addresses again when it connects and connects to the one it checked, so a name that resolves to a
public address for the check and a LAN one for the fetch never reaches the LAN.
"""

import http.client
import ipaddress
import socket
import urllib.parse
import urllib.request


def public(addr) -> bool:
    """Whether an address is on the public internet: not private, loopback, link-local, shared
    (100.64.0.0/10, a tailnet's range), reserved or multicast."""
    return addr.is_global and not addr.is_multicast


def resolve(host: str, port=None) -> list:
    """getaddrinfo for host, or a refusal (OSError) when any of its addresses isn't public."""
    try:
        infos = socket.getaddrinfo(host, port, type=socket.SOCK_STREAM)
    except socket.gaierror:
        raise OSError("could not resolve that host")
    for info in infos:
        if not public(ipaddress.ip_address(info[4][0].split("%")[0])):
            raise OSError("that address is not public")
    return infos


def safe_url(url: str):
    """Return (parsed, None) if the URL is safe to fetch, else (None, reason).

    Checks every resolved address, not the hostname, so a name pointing at a
    private range is refused too.
    """
    try:
        p = urllib.parse.urlparse(url)
    except ValueError:
        return None, "that is not a url"
    if p.scheme not in ("http", "https"):
        return None, "only http and https links"
    if not p.hostname:
        return None, "that url has no host"
    try:
        resolve(p.hostname)
    except OSError as e:
        return None, str(e)
    return p, None


def connect(address, timeout=socket._GLOBAL_DEFAULT_TIMEOUT, source_address=None, *args):
    """socket.create_connection, to an address resolve() has just checked."""
    host, port = address
    err = None
    for family, kind, proto, _, sockaddr in resolve(host, port):
        sock = socket.socket(family, kind, proto)
        try:
            if timeout is not socket._GLOBAL_DEFAULT_TIMEOUT:
                sock.settimeout(timeout)
            if source_address:
                sock.bind(source_address)
            sock.connect(sockaddr)
            return sock
        except OSError as e:
            sock.close()
            err = e
    raise err or OSError("could not connect to that host")


class _HTTP(http.client.HTTPConnection):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._create_connection = connect


class _HTTPS(http.client.HTTPSConnection):
    # The certificate and SNI are still the hostname's: only the socket goes to the checked address.
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)
        self._create_connection = connect


class _HTTPHandler(urllib.request.HTTPHandler):
    def http_open(self, req):
        return self.do_open(_HTTP, req)


class _HTTPSHandler(urllib.request.HTTPSHandler):
    def https_open(self, req):
        return self.do_open(_HTTPS, req, context=self._context)


class NoRedirect(urllib.request.HTTPRedirectHandler):
    """Refuse redirects: a public URL could redirect to an address safe_url never checked."""

    def redirect_request(self, *args, **kwargs):
        return None


def opener():
    """An opener that connects only to public addresses, never follows a redirect, and ignores proxy
    settings (through a proxy, the address checked would be the proxy's)."""
    return urllib.request.build_opener(urllib.request.ProxyHandler({}), NoRedirect, _HTTPHandler, _HTTPSHandler)
