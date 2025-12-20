import json, socket, ssl, urllib.request, ipaddress, argparse, logging
from cryptography import x509
from cryptography.x509.oid import NameOID
from concurrent.futures import ThreadPoolExecutor, as_completed
from threading import Event
from tqdm import tqdm

GEO_URL = "https://stat.ripe.net/data/maxmind-geo-lite/data.json?resource={}"

logging.basicConfig(level=logging.INFO, format="%(asctime)s %(levelname)s %(message)s")
log = logging.getLogger(__name__)


def iter_ipv4_hosts(subnet):
    for ip in ipaddress.IPv4Network(subnet, strict=False).hosts():
        yield str(ip)


def domains_from_ip(ip, timeout=3):
    try:
        ctx = ssl._create_unverified_context()
        with socket.create_connection((ip, 443), timeout=timeout) as s:
            with ctx.wrap_socket(s) as ss:
                cert = x509.load_der_x509_certificate(ss.getpeercert(True))

        norm = lambda d: d.lower()[2:] if d.startswith("*.") else d.lower()

        return sorted(
            {
                norm(v)
                for a in cert.subject
                if a.oid == NameOID.COMMON_NAME
                for v in (a.value,)
            }
            | {
                norm(d)
                for ext in cert.extensions
                if isinstance(ext.value, x509.SubjectAlternativeName)
                for d in ext.value.get_values_for_type(x509.DNSName)
            }
        )

    except:
        return []


def is_domain_in_subnet(domain, subnet):
    try:
        ip = ipaddress.IPv4Address(socket.gethostbyname(domain))
        return ip in ipaddress.IPv4Network(subnet, strict=False)
    except:
        return False


def is_website_ok(domain, cors=False, timeout=3):
    try:
        req = urllib.request.Request(f"https://{domain}", method="HEAD")
        with urllib.request.urlopen(req, timeout=timeout) as r:
            return (not cors) or (r.headers.get("Access-Control-Allow-Origin") == "*")
    except:
        return False


def country_from_ip(ip, timeout=3):
    try:
        with urllib.request.urlopen(GEO_URL.format(ip), timeout=timeout) as r:
            j = json.load(r)
        return j["data"]["located_resources"][0]["locations"][0]["country"]
    except Exception:
        return None


def wfs_worker(subnet, ip, cors, stop):
    if stop.is_set():
        return {"status": "noop"}

    domains = domains_from_ip(ip)
    if not domains:
        return {"status": "empty"}

    found = []
    for d in domains:
        if not is_domain_in_subnet(d, subnet):
            continue
        log.info(f"{subnet} :: {ip} => {d} domain in subnet")

        if is_website_ok(d, cors=cors):
            log.warning(f"{subnet} :: {ip} => {d} website found")
            found.append({"domain": d, "ip": ip})

    if len(found) == 0:
        return {"status": "domains_only"}

    return {"status": "found", "data": found}


def websites_from_subnet(
    subnet, max_ok=None, max_fails=None, max_seq_fails=None, cors=False, workers=8
):
    log.info(f"{subnet} subnet checking...")
    ok = []
    seen_domains = set()
    fails = 0
    seq_fails = 0
    stop = Event()

    with ThreadPoolExecutor(max_workers=workers) as ex:
        futs = [
            ex.submit(wfs_worker, subnet, ip, cors, stop)
            for ip in iter_ipv4_hosts(subnet)
        ]

        for fut in as_completed(futs):
            res = fut.result()
            if res["status"] == "found":
                for x in res["data"]:
                    d = x["domain"]
                    if d in seen_domains:
                        continue
                    seen_domains.add(d)
                    ok.append(x)

            if res["status"] == "empty":
                fails += 1
                seq_fails += 1
                continue

            if res["status"] == "domains_only":
                seq_fails = 0
                continue

            if max_ok and len(seen_domains) >= max_ok:
                log.info(f"{subnet} max oks reached")
                stop.set()
                return ok

            if max_fails and fails >= max_fails:
                log.info(f"{subnet} max fails reached")
                stop.set()
                return ok

            if max_seq_fails and seq_fails >= max_seq_fails:
                log.info(f"{subnet} max seq fails reached")
                stop.set()
                return ok
    stop.set()
    return ok


def process_providers_websites(
    providers,
    max_ok_subnet=None,
    max_seq_fails_subnet=None,
    max_fails_subnet=None,
    max_ok_provider=None,
    cors=False,
    workers=8,
):
    result = []

    total_subnets = sum(len(p["subnets"]) for p in providers)
    pbar = tqdm(total=total_subnets, desc="subnets", unit="subnet")

    for p in providers:
        log.info(f"[{p["name"]}] provider started")
        items = []

        for idx, subnet in enumerate(p["subnets"]):
            if max_ok_provider and len(items) >= max_ok_provider:
                pbar.update(len(p["subnets"]) - idx)
                break

            found = websites_from_subnet(
                subnet,
                max_ok=max_ok_subnet,
                max_seq_fails=max_seq_fails_subnet,
                max_fails=max_fails_subnet,
                cors=cors,
                workers=workers,
            )

            for x in found:
                items.append(
                    {
                        "domain": x["domain"],
                        "ip": x["ip"],
                        "country": country_from_ip(x["ip"]),
                    }
                )

            pbar.update(1)

        log.info(f"[{p["name"]}] provider end (websites: {len(items)})")
        result.append({"provider": p["name"], "websites": items})

    pbar.close()
    return result


parser = argparse.ArgumentParser(
    description="Discover HTTPS websites inside provider subnets.",
    formatter_class=argparse.RawTextHelpFormatter,
)

parser.add_argument(
    "-i",
    "--input",
    metavar=":i",
    required=True,
    help="path to input .json file that specifies providers and their subnets",
)
parser.add_argument(
    "-o",
    "--output",
    metavar=":o",
    default="data/subnets2websites.json",
    help="path to output .json file (default: ./data/subnets2websites.json)",
)

parser.add_argument(
    "-mos",
    "--max-ok-subnet",
    metavar=":mos",
    type=int,
    default=None,
    help="max successful websites per subnet",
)

parser.add_argument(
    "-mfs",
    "--max-fails-subnet",
    metavar=":mfs",
    type=int,
    default=None,
    help="max fails to discover a website (per subnet)",
)

parser.add_argument(
    "-msfs",
    "--max-seq-fails-subnet",
    metavar=":msfs",
    type=int,
    default=None,
    help="max seq fails to discover a website (per subnet)",
)

parser.add_argument(
    "-mop",
    "--max-ok-provider",
    metavar=":mwp",
    type=int,
    default=None,
    help="max successful websites per provider (across all subnets)",
)


parser.add_argument(
    "-w",
    "--workers",
    metavar=":w",
    type=int,
    default=8,
    help="max parallel workers (per subnet; default: 8)",
)

parser.add_argument(
    "-c",
    "--cors",
    action="store_true",
    help="only HTTPS sites with any Access-Control-Allow-Origin",
)

args = parser.parse_args()

with open(args.input) as f:
    providers = json.load(f)

result = process_providers_websites(
    providers,
    max_ok_subnet=args.max_ok_subnet,
    max_seq_fails_subnet=args.max_seq_fails_subnet,
    max_fails_subnet=args.max_fails_subnet,
    max_ok_provider=args.max_ok_provider,
    cors=args.cors,
    workers=args.workers,
)

with open(args.output, "w") as f:
    json.dump(result, f, ensure_ascii=False, indent=2)
