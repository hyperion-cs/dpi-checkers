import json, urllib.request, ipaddress, argparse

AS_URL = "https://stat.ripe.net/data/announced-prefixes/data.json?resource={}"

def fetch_as_subnets(asn):
    with urllib.request.urlopen(AS_URL.format(asn)) as r:
        return [p["prefix"] for p in json.load(r)["data"]["prefixes"]]

def ipv4_subnets_dedup(subnets):
    nets = [ipaddress.IPv4Network(s, strict=False) for s in subnets if "." in s]
    return [str(n) for n in ipaddress.collapse_addresses(nets)]

def fetch_provider_subnets(prov):
    raw = {p for asn in prov["asns"] for p in fetch_as_subnets(asn)}
    return ipv4_subnets_dedup(raw)

parser = argparse.ArgumentParser(
    description="Fetch IPv4 subnets announced by providers ASNs.",
    formatter_class=argparse.RawTextHelpFormatter,
)

parser.add_argument(
    "-i",
    "--input",
    metavar=":i",
    required=True,
    help="path to input .json file that specifies providers and their ASNs",
)

parser.add_argument(
    "-o",
    "--output",
    metavar=":o",
    default="data/providers2subnets.json",
    help="path to output .json file (default: ./data/providers2subnets.json)",
)

args = parser.parse_args()

with open(args.input) as f:
    subnets = [{"name": p["name"], "subnets": fetch_provider_subnets(p)} for p in json.load(f)]

with open(args.output, "w") as f:
    json.dump(subnets, f, ensure_ascii=False, indent=2)
