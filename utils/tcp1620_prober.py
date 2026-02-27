# Requirements: dnspython, rich

import ipaddress, os, socket, string, random, ssl, time, argparse, threading
from types import SimpleNamespace
from concurrent.futures import ThreadPoolExecutor, as_completed
from itertools import product

import dns.resolver
from rich.table import Table
from rich.console import Console

THR_BYTES = 64 * 1024
RECV_BUF = 8 * 1024
FAKE_DOMAIN_LEN = 15  # without TLD
REQ_TIMEOUT = 15
SERVER_WAITS_CHECK_TIMEOUT = 5
DELAY_PER_TASK = 0.15
DNS_FETCH_DEPTH = 10

outgoing_traffic = 0
incoming_traffic = 0
checks_count = 0
traffic_lock = threading.Lock()


def update_stat(tx=0, rx=0, cc=0):
    global outgoing_traffic, incoming_traffic, checks_count
    with traffic_lock:
        outgoing_traffic += tx
        incoming_traffic += rx
        checks_count += cc


def prepare_http_reqline_and_headers(type, host_header, content_len):
    host_header_raw = b""
    if host_header is not None:
        if len(host_header) > 0:
            host_header_raw = b"Host: " + host_header.encode() + b"\r\n"
        else:
            host_header_raw = b"Host: \r\n"

    reqline_and_headers = (
        type.encode()
        + b" / HTTP/1.1\r\n"
        + host_header_raw
        + (
            b"Content-Length: " + str(content_len).encode() + b"\r\n"
            b"Connection: close\r\n\r\n"
        )
    )

    return reqline_and_headers


def incoming_stream_to_vacuum(s):
    l = 0
    while True:
        p = s.recv(RECV_BUF)
        if not p:
            break
        l += len(p)
    update_stat(0, l)
    return l


def do_http_request(ip, port, type, http_host_header, body_bytes, read=False):
    waits = False
    err = None
    try:
        sock = socket.create_connection((ip, port), timeout=REQ_TIMEOUT)
        reqline_and_headers = prepare_http_reqline_and_headers(
            type, http_host_header, len(body_bytes)
        )
        sock.sendall(reqline_and_headers)
        update_stat(len(reqline_and_headers), 0)
        waits = is_server_waits(sock)
        sock.sendall(body_bytes)
        update_stat(len(body_bytes), 0)
        if read:
            incoming_stream_to_vacuum(sock)

    except Exception as e:
        err = e

    sock.close()
    return (waits, err)


# determine if the server waits for the body before returning a response
def is_server_waits(s):
    waits = False
    buf = 1
    try:
        s.settimeout(SERVER_WAITS_CHECK_TIMEOUT)
        s.recv(buf)
    except Exception as e:
        waits = True
    finally:
        s.settimeout(REQ_TIMEOUT)

    return waits


def do_https_request(
    ip, port, type, http_host_header, sni, tls_v, body_bytes, read=False
):
    waits = False
    err = None

    try:
        ctx = ssl.create_default_context()
        ctx.check_hostname = False
        ctx.verify_mode = ssl.CERT_NONE
        ctx.minimum_version = tls_v
        ctx.maximum_version = tls_v

        sock = socket.create_connection((ip, port), timeout=REQ_TIMEOUT)
        tls = ctx.wrap_socket(sock, server_hostname=sni)

        reqline_and_headers = prepare_http_reqline_and_headers(
            type, http_host_header, len(body_bytes)
        )
        tls.sendall(reqline_and_headers)
        update_stat(len(reqline_and_headers), 0)
        waits = is_server_waits(tls)
        tls.sendall(body_bytes)
        update_stat(len(body_bytes), 0)
        if read:
            incoming_stream_to_vacuum(tls)
        tls.close()

    except Exception as e:
        err = e

    return (waits, err)


def handle_err(err):
    if isinstance(err, ssl.SSLError):
        return "tls err"
    if isinstance(err, OSError):
        return "conn err"
    return "internal err"


def do_head_post_seq(ip, port, http_host_header, tls=False, sni=None, tls_v=None):
    res = SimpleNamespace(
        ip=ip,
        port=port,
        http_host_header=http_host_header,
        tls=tls,
        sni=sni,
        tls_v=tls_v,
        alive=False,
        alive_err=None,
        dpi_detected=False,
        dpi_err=None,
        is_server_waits=False,
    )

    try:
        try:
            if tls:
                do_https_request(ip, port, "HEAD", http_host_header, sni, tls_v, b"")
            else:
                do_http_request(ip, port, "HEAD", http_host_header, b"")
            res.alive = True
        except socket.timeout:
            res.alive = False
        except Exception as e:
            res.alive_err = handle_err(e)

        if not res.alive:
            return

        try:
            body_bytes = os.urandom(THR_BYTES)
            if tls:
                res.is_server_waits, err = do_https_request(
                    ip, port, "POST", http_host_header, sni, tls_v, body_bytes, True
                )
                if err:
                    raise err

            else:
                res.is_server_waits, err = do_http_request(
                    ip, port, "POST", http_host_header, body_bytes, True
                )
                if err:
                    raise err

            res.dpi_detected = False
        except socket.timeout:
            res.dpi_detected = True
        except Exception as e:
            res.dpi_err = handle_err(e)
    finally:
        update_stat(0, 0, 1)
        return res


def lookup_ip(host):
    try:
        return str(ipaddress.ip_address(host))
    except:
        return socket.gethostbyname(host)


def fetch_dns_a_records(host, lookup):
    r = set([lookup])
    for _ in range(DNS_FETCH_DEPTH):
        # it is important to use a system default nameservers
        a_records = dns.resolver.resolve(host, "A")
        for a in a_records:
            r.add(str(a))
    return list(sorted(r))


def run_tasks(ip, host, fake_domain, progress_msg):
    tasks = []
    res = []

    tls_v_opts = [ssl.TLSVersion.TLSv1_2, ssl.TLSVersion.TLSv1_3]
    sni_opts = [host, fake_domain, None] if host != ip else [fake_domain, None]
    http_host_header_opts = (
        [host, ip, fake_domain, None] if host != ip else [ip, fake_domain, None]
    )
    with ThreadPoolExecutor() as ex:
        for http_host_header in http_host_header_opts:
            for sni, tls_v in product(sni_opts, tls_v_opts):
                time.sleep(DELAY_PER_TASK)
                tasks.append(
                    ex.submit(
                        do_head_post_seq, ip, 443, http_host_header, True, sni, tls_v
                    )
                )

            time.sleep(DELAY_PER_TASK)
            tasks.append(ex.submit(do_head_post_seq, ip, 443, http_host_header, False))
            time.sleep(DELAY_PER_TASK)
            tasks.append(ex.submit(do_head_post_seq, ip, 80, http_host_header, False))

        total = len(tasks)
        for i, fut in enumerate(as_completed(tasks), 1):
            progress_msg(f"{i/total:.1%}")
            res.append(fut.result())

    return res


def probe(a):
    dns_records = ""
    if a.ip:
        ip = a.ip
    else:
        ip = lookup_ip(a.host)
        if a.host != ip:
            dns_records = f"dns A records (depth={DNS_FETCH_DEPTH}):\n- {"\n- ".join(fetch_dns_a_records(a.host, ip))}\n\n"

    if a.timeout:
        REQ_TIMEOUT = a.timeout

    fake_domain = (
        "".join(
            random.choices(string.ascii_lowercase + string.digits, k=FAKE_DOMAIN_LEN)
        )
        + ".com"
    )

    print(
        f"host: {a.host}\n{dns_records}lookup ip: {ip}\nfake domain: {fake_domain}\n\n"
        + "* http host header in https mode is only important for server (censor cannot see it)\n"
        + '* "wfb" table header — did server wait for request body to be transmitted before response?\n'
    )

    progress_msg = lambda p: print(f"\rchecking... progress: {p}%", end="", flush=True)
    progress_msg(0)
    print(f"\r", end="", flush=True)

    t_results = run_tasks(ip, a.host, fake_domain, progress_msg)
    t_results.sort(key=lambda x: pretty_item_to_row(x, a.host, True))

    # drop progress bar
    print("\033[A\033[K")
    view_results(t_results, a.host)


def pretty_v(v):
    if v is None:
        return "[color(241)]n/a[/]"
    if v == "":
        return "[color(241)]empty but sent[/]"
    return v


def pretty_alive(x):
    if x.alive:
        return f"[color(82)]yes[/]"

    c = 135 if x.alive_err and "tls" in x.alive_err else 226
    return f"[color({c})]{x.alive_err}[/]" if x.alive_err else f"[color(160)]no[/]"


def pretty_dpi(x):
    if not x.alive:
        return "[color(241)]n/a[/]"
    if x.dpi_detected:
        return f"[color(160)]detected[/]"
    return f"[color(226)]{x.dpi_err}[/]" if x.dpi_err else f"[color(82)]not detected[/]"


def pretty_tls_v(x):
    if x.tls_v is None:
        return "[color(241)]n/a[/]"
    if x.tls_v == ssl.TLSVersion.TLSv1_2:
        return "v1.2"
    if x.tls_v == ssl.TLSVersion.TLSv1_3:
        return "v1.3"
    return x.name


def pretty_proto(x):
    if x.tls:
        return "https"
    if x.port == 443:
        return "http over https"
    return "http"


def pretty_waits(x):
    return "[color(82)]yes[/]" if x.is_server_waits else "[color(135)]no[/]"


def set_color_if(s, c):
    for color, cond in c:
        if cond(s):
            # ansi 256 color
            return f"[color({color})]{s}[/]"
    return s


def pretty_item_to_row(x, host, sorting=False):
    proto = pretty_proto(x)
    port = str(x.port)
    if sorting:
        proto = 1 if proto == "http" else (2 if proto == "http over https" else 3)
        port = 1 if port == 80 else 2

    return (
        f"[color({241})]{x.ip}[/]",
        port,
        proto,
        pretty_tls_v(x),
        set_color_if(
            pretty_v(x.sni),
            [
                (39, lambda o: o == host),
            ],
        ),
        set_color_if(pretty_v(x.http_host_header), [(39, lambda o: o == host)]),
        pretty_alive(x),
        pretty_waits(x),
        pretty_dpi(x),
    )


def view_results(res, host):
    table = Table()
    table.add_column("dst", justify="center")
    table.add_column("port", justify="center")
    table.add_column("proto", justify="center")
    table.add_column("tls", justify="center")
    table.add_column("sni", justify="center")
    table.add_column("http host header", justify="center")
    table.add_column("alive", justify="center")
    table.add_column("wfb", justify="center")
    table.add_column("dpi", justify="center")

    for x in res:
        table.add_row(*pretty_item_to_row(x, host))
    Console().print(table)


p = argparse.ArgumentParser(
    description="checks comprehensively if there are any restrictions using tcp 16-20 method to host"
)
p.add_argument(
    "--host",
    metavar=":host",
    type=str,
    default="google.com",
    help="domain or ip to check (def: google.com)",
    required=True,
)

p.add_argument(
    "-t",
    "--timeout",
    metavar=":t",
    type=int,
    default=20,
    help="request timeout in sec (def: 15 sec)",
)

p.add_argument(
    "--ip",
    metavar=":ip",
    type=str,
    default=None,
    help="resolve host manually to this ip (def: none)",
)

probe(p.parse_args())
print(
    (
        f"done; checks: {checks_count}, "
        f"tcp tx: {outgoing_traffic/1024:.2f}KB, "
        f"rx: {incoming_traffic/1024:.2f}KB"
    )
)
