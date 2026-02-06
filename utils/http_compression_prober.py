# Requirements: brotli, zstandard

import argparse
import requests
import urllib3
import zlib, brotli, zstandard

urllib3.disable_warnings(urllib3.exceptions.InsecureRequestWarning)


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


def probe_url(url, decompr_name, user_agent, compr_min, decompr_chunk):
    cl, dl = 0, 0

    r = requests.get(
        url,
        headers={"Accept-Encoding": decompr_name, "User-Agent": user_agent},
        stream=True,
        allow_redirects=False,
        verify=False,
    )
    r.raw.decode_content = False

    if "Content-Encoding" not in r.headers:
        print(f"{decompr_name}: not supported by endpoint")
        return

    decompress, flush = get_decompr(decompr_name)
    while cl < compr_min or compr_min == -1:
        b = r.raw.read(decompr_chunk)
        if not b:
            if compr_min != -1:
                print(f"{decompr_name}: EOF before :min")
            break
        cl += len(b)
        dl += len(decompress(b))

    dl += len(flush())
    print(f"{decompr_name}: compr={cl} decompr={dl} (x{(dl / cl):.2f})")


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
    default=24 * 1024,
    help="minimum compressed data in bytes (no limit if -1; def: 24KB)",
)

p.add_argument(
    "--chunk",
    metavar=":chunk",
    type=int,
    default=1024,
    help="chunk size in http stream (def: 1KB)",
)

a = p.parse_args()
for decompr in ("gzip", "deflate", "br", "zstd"):
    probe_url(a.url, decompr, a.ua, a.min, a.chunk)
