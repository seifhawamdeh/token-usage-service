package identity

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
)

// SourceID is a deterministic id from host + vendor + absolute path.
func SourceID(hostID, vendor, sourcePath string) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("v1\x00%s\x00%s\x00%s", hostID, vendor, sourcePath)))
	return hex.EncodeToString(h[:])
}

// ProcessingSignature invalidates watermarks when adapter/schema interpretation changes.
func ProcessingSignature(adapterName, adapterVersion string, schemaVersion int) string {
	return fmt.Sprintf("%s@%s|schema=%d", adapterName, adapterVersion, schemaVersion)
}
