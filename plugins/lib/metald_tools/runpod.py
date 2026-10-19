# Copyright (C) 2026 MysteryGodzilla
# Part of Mizira, a fork of metald (github.com/B4reMetal/metald)
# SPDX-License-Identifier: GPL-3.0-only
"""Run a ComfyUI workflow on a RunPod serverless endpoint running worker-comfyui.

    from metald_tools import runpod
    out = runpod.run(workflow, timeout=600, log=my_logger)   # {"images": [bytes, ...], "texts": [...]}

RUNPOD_API_KEY and RUNPOD_ENDPOINT_ID pick the endpoint. Failures raise comfyui.ComfyError with a
message safe to show in a channel; the detail goes to the operator log.
"""

import base64
import json
import os
import time
import urllib.error
import urllib.request

from metald_tools import toollog
from metald_tools.comfyui import ComfyCancelled, ComfyError

API = os.environ.get("RUNPOD_API_URL", "https://api.runpod.ai/v2").rstrip("/")
POLL_INTERVAL = 2
# api.runpod.ai sits behind Cloudflare, which refuses Python's default user agent.
USER_AGENT = "metald-runpod/1"


def requires() -> list:
    return ["RUNPOD_API_KEY", "RUNPOD_ENDPOINT_ID"]


def _log(log):
    return log or (lambda m: toollog.log_detail("runpod", m))


def _call(method: str, path: str, body=None, timeout: float = 30):
    url = f"{API}/{os.environ.get('RUNPOD_ENDPOINT_ID', '')}{path}"
    req = urllib.request.Request(url, method=method,
                                 data=json.dumps(body).encode("utf-8") if body is not None else None,
                                 headers={"Authorization": "Bearer " + os.environ.get("RUNPOD_API_KEY", ""),
                                          "Content-Type": "application/json", "User-Agent": USER_AGENT})
    with urllib.request.urlopen(req, timeout=timeout) as resp:
        return json.loads(resp.read())


def workers():
    """The endpoint's worker counts from /health ({"idle", "running", "initializing", ...}), or None."""
    try:
        return _call("GET", "/health", timeout=10).get("workers") or {}
    except (urllib.error.URLError, OSError, ValueError):
        return None


def cold() -> bool:
    """Whether a job has to start a worker first: none is running. A scaled-down worker restarts in
    seconds but loads the model again, so the job takes twice as long or more; one still being set up
    takes minutes. Unknown counts as warm, so a failed check never claims a wait."""
    w = workers()
    if w is None:
        return False
    return not w.get("running", 0)


def _decode(item) -> bytes:
    data = item.get("data", "") if isinstance(item, dict) else ""
    if "," in data[:100]:
        data = data.split(",", 1)[1]
    return base64.b64decode(data)


def run(workflow: dict, timeout: float = 600, log=None, inputs: dict = None) -> dict:
    """Submit an API-format workflow ({node id: node}), wait, and return its images (bytes) and
    texts. inputs ({name: bytes}) are put in ComfyUI's input folder first, for LoadImage nodes. A job
    still waiting at the timeout is cancelled, so it can't run and bill later."""
    log = _log(log)
    job_input = {"workflow": workflow}
    if inputs:
        job_input["images"] = [{"name": n, "image": base64.b64encode(b).decode("ascii")} for n, b in inputs.items()]
    try:
        job = _call("POST", "/run", {"input": job_input})
    except urllib.error.HTTPError as e:
        log(f"runpod refused the job: {e.code} {e.read().decode('utf-8', 'replace')[:300]}")
        raise ComfyError("backend unreachable")
    except (urllib.error.URLError, OSError, ValueError) as e:
        log(f"runpod unreachable: {e}")
        raise ComfyError("backend unreachable")
    job_id = job.get("id", "")
    started = time.time()
    status = {}
    while time.time() - started < timeout:
        time.sleep(POLL_INTERVAL)
        try:
            status = _call("GET", f"/status/{job_id}")
        except (urllib.error.URLError, OSError, ValueError) as e:
            log(f"status poll failed: {e}")
            continue
        state = status.get("status")
        if state == "COMPLETED":
            break
        if state == "CANCELLED":
            raise ComfyCancelled("cancelled")
        if state in ("FAILED", "TIMED_OUT"):
            log(f"job {job_id} {state}: {json.dumps(status.get('error') or status.get('output'))[:600]}")
            raise ComfyError("generation failed")
    else:
        try:
            _call("POST", f"/cancel/{job_id}")
        except (urllib.error.URLError, OSError, ValueError) as e:
            log(f"cancel failed: {e}")
        log(f"job {job_id} timed out after {timeout:.0f}s ({status.get('status', 'no status')})")
        raise ComfyError("generation timed out")

    out = status.get("output") or {}
    # Waiting for a worker plus running the workflow; execution is the billed GPU time.
    log(f"job {job_id} done: delay_ms={status.get('delayTime', 0)} execution_ms={status.get('executionTime', 0)} "
        f"gpu_seconds={status.get('executionTime', 0) / 1000:.1f}")
    if out.get("errors"):
        log(f"job {job_id} warnings: {json.dumps(out['errors'])[:600]}")
    try:
        images = [_decode(i) for i in out.get("images") or []]
    except ValueError as e:
        log(f"job {job_id} returned an unreadable image: {e}")
        raise ComfyError("could not read the generated output")
    if not images:
        log(f"job {job_id} returned no image: {json.dumps(out)[:600]}")
        raise ComfyError("generation produced no images")
    return {"images": images, "texts": [t for t in out.get("texts") or [] if isinstance(t, str)]}
