package bridge

import (
	"encoding/hex"
	"strings"
)

func IsValidRepoName(repoName string) bool {
	return len(repoName) > 0 && !strings.ContainsAny(repoName, " /.")
}

// IsCorruptedRepo checks if a repository event is corrupted and should be rejected
// This prevents corrupted repos from being stored in the database or filesystem
func IsCorruptedRepo(eventID string, repoName string, pubkey string) bool {
	// Check for empty repo name
	if repoName == "" || strings.TrimSpace(repoName) == "" {
		return true
	}

	// Pubkey should be a valid hex string (64 chars for 32 bytes)
	if len(pubkey) != 64 {
		return true
	}

	// Try to decode pubkey as hex to validate format
	_, err := hex.DecodeString(pubkey)
	if err != nil {
		return true // Invalid pubkey format
	}

	return false
}
