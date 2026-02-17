# Requirements: brotli, zstandard

import argparse
import requests
from http import HTTPStatus
import urllib3
import zlib, brotli, zstandard

urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)

VERDICT = "verdict"
VERDICT__OK = "ok"
VERDICT__NOT_SUPPORTED = "not supported"
VERDICT__EOF_BEFORE_MIN = "eof before min"
VERDICT__TIMEOUT = "timeout"
VERDICT__CONN_ERR = "connection error"
VERDICT__INTERNAL_ERR = "internal error"
HTTP_STATUS = "http status"
COMPR = "compr"
DECOMPR = "decompr"
NAME = "name"


def get_decompr(name):
    if name == "gzip":
        d = zlib.decompressobj(16 + zlib.MAX_WBITS)
        return (d.decompress, d.flush)

    if name == "deflate":
        d = zlib.decompressobj()
        return (d.decompress, d.flush)

    if name == "br":
        d = brotli.Decompressor()
        return (d.process, lambda: b"")

    if name == "zstd":
        d = zstandard.ZstdDecompressor().decompressobj()
        return (d.decompress, d.flush)

    return ValueError(name)


def probe_url(url, decompr_name, user_agent, compr_min, decompr_chunk, timeout):
    rslt = {NAME: decompr_name, COMPR: 0, DECOMPR: 0}

    try:
        resp = requests.get(
            url,
            headers={"Accept-Encoding": decompr_name, "User-Agent": user_agent},
            stream=True,
            allow_redirects=False,
            verify=False,
            timeout=timeout,
        )
        resp.raw.decode_content = False
        rslt[HTTP_STATUS] = resp.status_code

        if "Content-Encoding" not in resp.headers:
            rslt[VERDICT] = VERDICT__NOT_SUPPORTED
            return rslt

        decompress, flush = get_decompr(decompr_name)
        while rslt[COMPR] < compr_min or compr_min == -1:
            b = resp.raw.read(decompr_chunk)
            if not b:
                if compr_min != -1:
                    rslt[VERDICT] = VERDICT__EOF_BEFORE_MIN
                break

            rslt[COMPR] += len(b)
            rslt[DECOMPR] += len(decompress(b))

        rslt[DECOMPR] += len(flush())
        rslt[VERDICT] = VERDICT__OK

    except (requests.exceptions.Timeout, urllib3.exceptions.TimeoutError):
        rslt[VERDICT] = VERDICT__TIMEOUT

    except (requests.exceptions.ConnectionError, urllib3.exceptions.ConnectionError):
        rslt[VERDICT] = VERDICT__CONN_ERR

    except:
        rslt[VERDICT] = VERDICT__INTERNAL_ERR

    return rslt


def start(a):
    bestCompr = None
    netErrs = False
    internalErrs = False
    notHttpOks = False

    for decompr in ("gzip", "deflate", "br", "zstd"):
        r = probe_url(a.url, decompr, a.ua, a.min, a.chunk, a.timeout)

        if r[VERDICT] == VERDICT__TIMEOUT:
            netErrs = True
            print(f"{decompr}: request timeout")
            continue

        if r[VERDICT] == VERDICT__CONN_ERR:
            netErrs = True
            print(f"{decompr}: connection error")
            continue

        if r[VERDICT] == VERDICT__INTERNAL_ERR:
            internalErrs = True
            print(f"{decompr}: internal error")
            continue

        if r[HTTP_STATUS] != HTTPStatus.OK:
            notHttpOks = True

        if r[VERDICT] == VERDICT__NOT_SUPPORTED:
            print(f"{decompr}: not supported by endpoint, http status={r[HTTP_STATUS]}")
            continue

        if r[VERDICT] in (VERDICT__OK, VERDICT__EOF_BEFORE_MIN):
            if bestCompr is None or r[DECOMPR] > bestCompr[DECOMPR]:
                bestCompr = r

            if r[VERDICT] == VERDICT__EOF_BEFORE_MIN:
                print(f"{decompr}: EOF before :min")

            print(
                f"{decompr}: compr={r[COMPR]}, decompr={r[DECOMPR]} (x{(r[DECOMPR] / r[COMPR]):.2f}), http status={r[HTTP_STATUS]}"
            )

    print()

    if netErrs:
        print("* network error detected")

    if internalErrs:
        print("* internal error detected")

    if notHttpOks:
        print("* a response other than HTTP OK (200) is detected")

    if bestCompr is None:
        print("* no compression methods detected")

    if bestCompr is not None:
        print(f"* best compression: {bestCompr[NAME]}")


p = argparse.ArgumentParser(
    description="Tests the efficiency of various compression options for a given HTTP endpoint with incoming traffic limit option."
)
p.add_argument("--url", metavar=":url", required=True, help="endpoint url")

p.add_argument(
    "--ua",
    metavar=":url",
    help="user-agent (def: google chrome)",
    default="Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/144.0.0.0 Safari/537.36",
)

p.add_argument(
    "--min",
    metavar=":min",
    type=int,
    default=64 * 1024,
    help="minimum compressed data in bytes (no limit if -1; def: 64KB)",
)

p.add_argument(
    "--chunk",
    metavar=":chunk",
    type=int,
    default=1024,
    help="chunk size in http stream (def: 1KB)",
)

p.add_argument(
    "-t",
    "--timeout",
    metavar=":t",
    type=int,
    default=15,
    help="request timeout in sec (def: 15 sec)",
)

start(p.parse_args())
