/*
const tryHandleShare = () => {
  const params = new URLSearchParams(window.location.search);
  const result = params.get("share");
  if (result) {
    const base64 = Uint8Array.fromBase64(result, { alphabet: "base64url" });
    console.log(base64);
    return true;
  }
  return false;
};
*/

const nowUtcToNum = (epoch) => BigInt(Math.floor((Date.now() - epoch) / 60000));

const encodeShare = async () => {
  // экодер всегда актуальный, поэтому он берет штатный файл helpers.js
  const h = await import('./helpers.js');


  const commitHex = "88315c44a98db097299a9d65193cfaef";
  const commit = BigInt("0x" + commitHex);
  const ts = nowUtcToNum(h.EPOCH_MS);
  const asn = BigInt(24940)

  const bufSize = Math.ceil((h.COMMIT_BITS + h.TIMESTAMP_BITS + h.ASN_BITS) / 8)
  const buf = new Uint8Array(bufSize);
  h.writeBits(buf, 0, h.COMMIT_BITS, commit);
  h.writeBits(buf, h.COMMIT_BITS, h.TIMESTAMP_BITS, ts);
  h.writeBits(buf, h.COMMIT_BITS + h.TIMESTAMP_BITS, h.ASN_BITS, asn);
  const base64 = buf.toBase64({ alphabet: "base64url", omitPadding: true })
  console.log("encoded:", base64)
}
