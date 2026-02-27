const nowUtcToBigint = (epoch) => BigInt(Math.floor((Date.now() - epoch) / 60000));

const encodeItem = (aliveCardinality, alive, dpi) => {
  return BigInt(alive + aliveCardinality * dpi)
};

const encodeShare = async (clientAsn, items) => {
  // encoder always takes latest file
  const h = await import('./helpers.js');
  const commit = await h.getLastCommitBigint();
  const ts = nowUtcToBigint(h.EPOCH_MS);
  const asn = BigInt(clientAsn)

  const sortedItems = Object.keys(items).sort().map(k => items[k]);
  const itemsTotalBits = h.ENDPOINT_STATE_BITS * sortedItems.length;
  const bufSize = Math.ceil((h.COMMIT_BITS + h.TIMESTAMP_BITS + h.ASN_BITS + itemsTotalBits) / 8)
  const buf = new Uint8Array(bufSize);

  h.writeBits(buf, 0, h.COMMIT_BITS, commit);
  h.writeBits(buf, h.COMMIT_BITS, h.TIMESTAMP_BITS, ts);
  h.writeBits(buf, h.COMMIT_BITS + h.TIMESTAMP_BITS, h.ASN_BITS, asn);

  let itemOffset = h.COMMIT_BITS + h.TIMESTAMP_BITS + h.ASN_BITS;
  for (const x of sortedItems) {
    const encodedItem = encodeItem(h.ALIVE_CARDINALITY, x.alive, x.dpi);
    h.writeBits(buf, itemOffset, h.ENDPOINT_STATE_BITS, encodedItem);
    itemOffset += h.ENDPOINT_STATE_BITS;
  }

  const base64 = buf.toBase64({ alphabet: "base64url", omitPadding: true })
  console.log("encoded share:", base64)
  return base64;
}
