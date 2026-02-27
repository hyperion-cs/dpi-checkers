export const COMMIT_BITS = 128;
export const TIMESTAMP_BITS = 23;
export const ASN_BITS = 32;
export const EPOCH_MS = Date.UTC(2026, 0, 1);

export const ENDPOINT_STATE_BITS = 4;
export const ALIVE_CARDINALITY = 3;
export const DPI_CARDINALITY = 5;

export const SELFCHECK_ID = "US.GH-HPRN";

export const getCommitHex = (buf) => {
  const commit = readBits(buf, 0, COMMIT_BITS)
  return commit.toString(16);
}

// Write BigInt to Uint8Array
export const writeBits = (buf, bitOffset, bitLength, value) => {
  for (let i = 0; i < bitLength; i++) {
    const bit = Number((value >> BigInt(i)) & 1n);
    const idx = bitOffset + bitLength - 1 - i;
    const byteIndex = idx >> 3;
    const bitIndex = 7 - (idx & 7);
    if (bit) {
      buf[byteIndex] |= 1 << bitIndex;
    } else {
      buf[byteIndex] &= ~(1 << bitIndex);
    }
  }
};

// Read Uint8Array to BigInt
export const readBits = (buf, bitOffset, bitLength) => {
  let result = 0n;
  for (let i = 0; i < bitLength; i++) {
    const idx = bitOffset + i;
    const byteIndex = idx >> 3;
    const bitIndex = 7 - (idx & 7);
    const bit = (buf[byteIndex] >> bitIndex) & 1;
    result = (result << 1n) | BigInt(bit);
  }
  return result;
};
