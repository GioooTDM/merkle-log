## On-chain payload (binary)
We store the following byte sequence:

`PNOT | version | tree_size | root_hash | checkpoint_hash`

### Field definitions
- **PNOT** (4 bytes, ASCII)
  - Purpose: application/log identifier (domain separation).
  - Value: the ASCII bytes of `"PNOT"`.
  - Note: this can be treated as a log-id/app-id prefix. If multiple logs exist, either:
    - use different 4-byte prefixes per log, or
    - keep `PNOT` fixed and add a separate `log_id` field (not used in this minimal format).

- **version** (1 byte, unsigned)
  - Purpose: allow future format evolution.
  - Initial value: `0x01`.

- **tree_size** (8 bytes, uint64, big-endian)
  - Meaning: number of leaves in the log at the checkpoint (the “tree size” in STH / checkpoint).

- **root_hash** (32 bytes, raw)
  - Meaning: Merkle root of the log at `tree_size`.
  - Encoding: raw bytes (not base64).

- **checkpoint_hash** (32 bytes)
  - Meaning: `SHA-256(checkpoint_text_bytes)`
  - Purpose: binds the *exact* checkpoint text (including signature line) to prevent ambiguity and to allow later re-validation.

## Checkpoint text hashing (canonicalization)
`checkpoint_text_bytes` MUST be defined unambiguously to avoid different hashes for the “same” checkpoint.

## Verification procedure
Given an on-chain payload and an off-chain checkpoint text:
1. Decode payload fields and read `tree_size`, `root_hash`, `checkpoint_hash`.
2. Canonicalize the off-chain checkpoint text and compute `SHA-256`.
3. Verify computed hash equals `checkpoint_hash`.
4. Optionally verify:
   - the checkpoint signature (off-chain),
   - that `tree_size` and `root_hash` extracted from the text match the payload.

## Notes / trade-offs
- This format is compact and blockchain-independent.
- It does not store the checkpoint text on-chain, only its digest.