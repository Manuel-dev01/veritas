package contract

import "crypto/sha256"

/*
state.go — Veritas state keys and deterministic id derivation.

All Veritas records live in the shared FSM key-value store under plugin-owned single-byte
prefixes. Canopy reserves prefixes 1-15 for core state and 16-23 for future use, so Veritas
uses 24+. Keys are built with JoinLenPrefix (defined in plugin.go) for unambiguous segment
boundaries. Determinism rule: ids are derived only from signed-transaction fields (never from
time/rand), so every validator computes byte-identical keys.
*/

var (
	claimPrefix     = []byte{24} // claim/{claimId}
	notePrefix      = []byte{25} // note/{noteId}
	ratingPrefix    = []byte{26} // rating/{noteId}/{rater}
	repPrefix       = []byte{27} // rep/{account}
	noteIndexPrefix = []byte{28} // noteIndexByClaim/{claimId}/{noteId} -> presence marker
	dirtyNotePrefix = []byte{29} // notesNeedingScore/{noteId} -> dirty-set marker
)

// KeyForClaim returns the state key for a claim record.
func KeyForClaim(claimID []byte) []byte { return JoinLenPrefix(claimPrefix, claimID) }

// KeyForNote returns the state key for a note record.
func KeyForNote(noteID []byte) []byte { return JoinLenPrefix(notePrefix, noteID) }

// KeyForRating returns the state key for one rater's rating of a note.
func KeyForRating(noteID, rater []byte) []byte { return JoinLenPrefix(ratingPrefix, noteID, rater) }

// KeyForNoteIndex returns the enumeration-index key linking a claim to one of its notes.
func KeyForNoteIndex(claimID, noteID []byte) []byte {
	return JoinLenPrefix(noteIndexPrefix, claimID, noteID)
}

// KeyForDirtyNote returns the dirty-set key marking a note as needing a rescore in EndBlock.
func KeyForDirtyNote(noteID []byte) []byte { return JoinLenPrefix(dirtyNotePrefix, noteID) }

// KeyForRep returns the state key for an account's reputation record.
func KeyForRep(account []byte) []byte { return JoinLenPrefix(repPrefix, account) }

// dirtyNoteScanPrefix / ratingScanPrefix return the encoded key prefix for a prefix range-scan over
// the whole record family (JoinLenPrefix of a single byte, e.g. {1,29} and {1,26}).
func dirtyNoteScanPrefix() []byte { return JoinLenPrefix(dirtyNotePrefix) }
func ratingScanPrefix() []byte    { return JoinLenPrefix(ratingPrefix) }
func repScanPrefix() []byte       { return JoinLenPrefix(repPrefix) }

// splitLenPrefix is the inverse of JoinLenPrefix: it splits a key back into its length-prefixed
// segments. For a dirty-note key (prefix ‖ noteId) the note id is the last segment. Returns nil on
// a malformed key (a truncated segment), so callers should check for the expected segment count.
func splitLenPrefix(key []byte) [][]byte {
	var segs [][]byte
	for i := 0; i < len(key); {
		n := int(key[i])
		i++
		if i+n > len(key) {
			return nil // malformed: declared length runs past the key
		}
		segs = append(segs, key[i:i+n])
		i += n
	}
	return segs
}

// DeriveClaimID deterministically derives a claim id from signed-tx fields:
// sha256(submitter ‖ nonce(8B big-endian) ‖ createdHeight(8B big-endian)). 32 bytes.
func DeriveClaimID(submitter []byte, nonce, createdHeight uint64) []byte {
	h := sha256.New()
	h.Write(submitter)
	h.Write(formatUint64(nonce))
	h.Write(formatUint64(createdHeight))
	return h.Sum(nil)
}

// DeriveNoteID deterministically derives a note id (used from Phase 2):
// sha256(author ‖ claimId ‖ nonce(8B BE) ‖ createdHeight(8B BE)). 32 bytes.
func DeriveNoteID(author, claimID []byte, nonce, createdHeight uint64) []byte {
	h := sha256.New()
	h.Write(author)
	h.Write(claimID)
	h.Write(formatUint64(nonce))
	h.Write(formatUint64(createdHeight))
	return h.Sum(nil)
}
