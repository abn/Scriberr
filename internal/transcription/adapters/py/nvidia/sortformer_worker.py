#!/usr/bin/env python3
"""
Persistent NVIDIA Sortformer diarization worker.
Loads the Sortformer .nemo model once, then accepts newline-delimited JSON
requests on stdin and writes newline-delimited JSON responses on stdout.
"""

import argparse
import contextlib
import json
import os
from pathlib import Path
import sys
import traceback

import torch

from nemo.collections.asr.models import SortformerEncLabelModel
from sortformer_diarize import save_results


def send(message):
    sys.stdout.write(json.dumps(message, separators=(",", ":")) + "\n")
    sys.stdout.flush()


def resolve_device(device):
    if device == "auto":
        return "cuda" if torch.cuda.is_available() else "cpu"
    if device == "cuda" and not torch.cuda.is_available():
        print("CUDA requested but not available, using CPU", file=sys.stderr)
        return "cpu"
    return device


def model_path_from_virtual_env():
    virtual_env = os.environ.get("VIRTUAL_ENV")
    if not virtual_env:
        raise RuntimeError("VIRTUAL_ENV is not set. Run this worker with 'uv run'.")

    project_root = os.path.dirname(virtual_env)
    return os.path.join(project_root, "diar_streaming_sortformer_4spk-v2.nemo")


def load_model(device):
    model_path = model_path_from_virtual_env()
    if not os.path.exists(model_path):
        raise FileNotFoundError(f"Sortformer model file not found: {model_path}")

    print(f"Loading persistent Sortformer model from: {model_path}", file=sys.stderr)
    diar_model = SortformerEncLabelModel.restore_from(
        restore_path=model_path,
        map_location=device,
        strict=False,
    )
    diar_model.eval()
    print(f"Persistent Sortformer model loaded on {device}", file=sys.stderr)
    return diar_model


def run_diarization(diar_model, request):
    audio_file = request["audio_file"]
    output_file = request["output_file"]
    output_format = request.get("output_format", "json")
    batch_size = int(request.get("batch_size") or 1)

    if not os.path.exists(audio_file):
        raise FileNotFoundError(f"Audio file not found: {audio_file}")

    Path(output_file).parent.mkdir(parents=True, exist_ok=True)
    predicted_segments = diar_model.diarize(audio=audio_file, batch_size=batch_size)
    save_results(predicted_segments, output_file, audio_file, output_format)


def main():
    parser = argparse.ArgumentParser(description="Persistent Sortformer diarization worker")
    parser.add_argument("--device", choices=["cpu", "cuda", "auto"], default="auto")
    args = parser.parse_args()

    try:
        device = resolve_device(args.device)
        with contextlib.redirect_stdout(sys.stderr):
            diar_model = load_model(device)
        send({"type": "ready", "ok": True, "model_id": "sortformer", "device": device})
    except Exception as exc:
        traceback.print_exc(file=sys.stderr)
        send({"type": "error", "ok": False, "model_id": "sortformer", "error": str(exc)})
        return 1

    for line in sys.stdin:
        line = line.strip()
        if not line:
            continue

        request = {}
        try:
            request = json.loads(line)
            request_id = request.get("id")
            action = request.get("action")

            if action == "shutdown":
                send({"type": "response", "id": request_id, "ok": True})
                return 0

            if action != "diarize":
                raise ValueError(f"Unsupported action: {action}")

            with contextlib.redirect_stdout(sys.stderr):
                run_diarization(diar_model, request)
            send({"type": "response", "id": request_id, "ok": True})
        except Exception as exc:
            traceback.print_exc(file=sys.stderr)
            send({
                "type": "response",
                "id": request.get("id"),
                "ok": False,
                "error": str(exc),
            })

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
