import argparse, os, socket, ssl, time

p = argparse.ArgumentParser(
    description="checks if there are any restrictions using l4-25 method to host"
)

p.add_argument("--host", required=True, help="domain or ip to check")
p.add_argument("--total", type=int, default=64, help="total payload bytes after establishing an https connection (def: 64)")
p.add_argument("--chunk", type=int, default=2, help="chunk size when sending the payload in bytes (def: 2)")
p.add_argument("--delay", type=int, default=50, help="delay between chucks in ms (def: 50)")
p.add_argument("--timeout", type=int, default=5000, help="response timeout in ms (def: 5000)")
a = p.parse_args()

RESP_BUF_SIZE = 4096

def headers(body_len):
    return (
        f"POST / HTTP/1.1\r\n"
        f"Host: {a.host}\r\n"
        f"Content-Length: {body_len}\r\n\r\n"
    ).encode()

body_len = a.total
while True:
    h = headers(body_len)
    n = a.total - len(h)
    if n == body_len:
        break
    body_len = n

req = h + os.urandom(max(0, body_len))

raw = socket.create_connection((a.host, 443))
raw.setsockopt(socket.IPPROTO_TCP, socket.TCP_NODELAY, 1)

ctx = ssl._create_unverified_context()
s = ctx.wrap_socket(raw, server_hostname=a.host)

total_chunks = (len(req) + a.chunk - 1) // a.chunk
sent_bytes = 0

for i in range(0, len(req), a.chunk):
    chunk = req[i:i + a.chunk]
    num = i // a.chunk + 1

    s.sendall(chunk)
    sent_bytes += len(chunk)
    print(f"\rsent #{num}/{total_chunks}: {sent_bytes}/{len(req)} bytes", end="", flush=True)

    time.sleep(a.delay / 1000)

print("\nall packets sent to the local OS network stack")

s.settimeout(a.timeout / 1000)

try:
    data = b""
    while b"\r\n" not in data:
        data += s.recv(RESP_BUF_SIZE)

    print("response received")
except socket.timeout:
    print("response timeout")

s.close()
