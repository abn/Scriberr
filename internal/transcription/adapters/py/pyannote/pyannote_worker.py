#!/usr/bin/env python3
"""
Persistent PyAnnote diarization worker.
Loads the PyAnnote pipeline once, then accepts newline-delimited JSON requests
on stdin and writes newline-delimited JSON responses on stdout.
"""

import argparse
import contextlib
import copy
import gc
import json
import os
from pathlib import Path
import sys
import traceback

from pyannote.audio import Pipeline
import torch

from pyannote_diarize import save_json_format


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


def reserve_cuda_vram(reserve_mb, device):
    if device != "cuda" or reserve_mb <= 0:
        return None

    reserve_bytes = int(reserve_mb) * 1024 * 1024
    reservation = torch.empty((reserve_bytes,), dtype=torch.uint8, device=torch.device("cuda"))
    torch.cuda.synchronize()
    print(f"Reserved {reserve_mb} MiB of CUDA VRAM for persistent PyAnnote", file=sys.stderr)
    return reservation


def release_cuda_vram(reservation):
    if reservation is None:
        return None

    del reservation
    gc.collect()
    torch.cuda.empty_cache()
    torch.cuda.synchronize()
    print("Released persistent PyAnnote CUDA VRAM reservation", file=sys.stderr)
    return None


def load_pipeline(args):
    print(f"Loading persistent PyAnnote pipeline: {args.model}", file=sys.stderr)
    pipeline = Pipeline.from_pretrained(args.model, token=args.hf_token)
    device = resolve_device(args.device)

    if device == "cuda":
        pipeline = pipeline.to(torch.device("cuda"))
        print("Using CUDA for persistent PyAnnote diarization", file=sys.stderr)
    else:
        print("Using CPU for persistent PyAnnote diarization", file=sys.stderr)

    return pipeline, device


def instantiate_for_request(pipeline, base_params, request):
    onset = request.get("segmentation_onset")
    offset = request.get("segmentation_offset")
    if onset is None and offset is None:
        return

    try:
        params = copy.deepcopy(base_params)
        if "segmentation" not in params:
            print("Warning: segmentation parameters not found in PyAnnote pipeline", file=sys.stderr)
            return

        if onset is not None:
            params["segmentation"]["threshold"] = float(onset)
        if offset is not None:
            params["segmentation"]["min_duration_off"] = float(offset)
        pipeline.instantiate(params)
    except Exception as exc:
        print(f"Warning: could not apply PyAnnote request thresholds: {exc}", file=sys.stderr)


def run_diarization(pipeline, base_params, request):
    audio_file = request["audio_file"]
    output_file = request["output_file"]
    output_format = request.get("output_format", "json")

    if not os.path.exists(audio_file):
        raise FileNotFoundError(f"Audio file not found: {audio_file}")

    Path(output_file).parent.mkdir(parents=True, exist_ok=True)
    instantiate_for_request(pipeline, base_params, request)

    diarization_params = {}
    if request.get("min_speakers"):
        diarization_params["min_speakers"] = int(request["min_speakers"])
    if request.get("max_speakers"):
        diarization_params["max_speakers"] = int(request["max_speakers"])

    if diarization_params:
        diarization = pipeline(audio_file, **diarization_params)
    else:
        diarization = pipeline(audio_file)

    if output_format == "rttm":
        with open(output_file, "w") as rttm:
            diarization.write_rttm(rttm)
    else:
        save_json_format(diarization, output_file, audio_file)


def main():
    parser = argparse.ArgumentParser(description="Persistent PyAnnote diarization worker")
    parser.add_argument("--hf-token", required=True)
    parser.add_argument("--model", default="pyannote/speaker-diarization-community-1")
    parser.add_argument("--device", choices=["cpu", "cuda", "auto"], default="auto")
    parser.add_argument("--reserve-vram-mb", type=int, default=0)
    args = parser.parse_args()

    reservation = None
    try:
        with contextlib.redirect_stdout(sys.stderr):
            pipeline, device = load_pipeline(args)
            base_params = copy.deepcopy(pipeline.parameters(instantiated=True))
            reservation = reserve_cuda_vram(args.reserve_vram_mb, device)
        send({"type": "ready", "ok": True, "model_id": "pyannote", "model": args.model})
    except Exception as exc:
        traceback.print_exc(file=sys.stderr)
        send({"type": "error", "ok": False, "model_id": "pyannote", "error": str(exc)})
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
                reservation = release_cuda_vram(reservation)
                run_diarization(pipeline, base_params, request)
                reservation = reserve_cuda_vram(args.reserve_vram_mb, device)
            send({"type": "response", "id": request_id, "ok": True})
        except Exception as exc:
            traceback.print_exc(file=sys.stderr)
            try:
                with contextlib.redirect_stdout(sys.stderr):
                    reservation = reserve_cuda_vram(args.reserve_vram_mb, device)
            except Exception as reserve_exc:
                print(f"Warning: could not reacquire PyAnnote VRAM reservation: {reserve_exc}", file=sys.stderr)
            send({
                "type": "response",
                "id": request.get("id"),
                "ok": False,
                "error": str(exc),
            })

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
