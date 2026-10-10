"""Serve the standalone design locally, including concurrent device previews."""

from functools import partial
from http.server import SimpleHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path
import sys


class PreviewServer(ThreadingHTTPServer):
    request_queue_size = 128
    daemon_threads = True


class PreviewHandler(SimpleHTTPRequestHandler):
    def log_message(self, format, *args):
        if len(args) > 1 and str(args[1]).startswith(("4", "5")):
            super().log_message(format, *args)


if __name__ == "__main__":
    port = int(sys.argv[1]) if len(sys.argv) > 1 else 8766
    handler = partial(PreviewHandler, directory=str(Path(__file__).parent))
    print(f"MeetSpace 01: http://127.0.0.1:{port}/", flush=True)
    try:
        PreviewServer(("127.0.0.1", port), handler).serve_forever()
    except KeyboardInterrupt:
        pass
