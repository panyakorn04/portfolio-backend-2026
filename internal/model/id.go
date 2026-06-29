package model

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"os"
	"sync/atomic"
	"time"
)

// The original database was created by Prisma, whose string IDs are cuid v1
// values (lowercase, prefixed with "c", base36). The columns are plain `text`
// with NO database default, so the application must supply an ID on insert.
//
// newID generates a cuid-compatible identifier so new rows blend in with the
// existing data and keep the same sortable, collision-resistant properties.

const cuidBlockSize = 4

var (
	cuidCounter     uint32
	cuidFingerprint = computeFingerprint()
)

func base36(n uint64, pad int) string {
	const alphabet = "0123456789abcdefghijklmnopqrstuvwxyz"
	if n == 0 {
		return padLeft("0", pad)
	}
	buf := make([]byte, 0, 16)
	for n > 0 {
		buf = append([]byte{alphabet[n%36]}, buf...)
		n /= 36
	}
	return padLeft(string(buf), pad)
}

func padLeft(s string, size int) string {
	for len(s) < size {
		s = "0" + s
	}
	if len(s) > size {
		return s[len(s)-size:]
	}
	return s
}

func computeFingerprint() string {
	pid := os.Getpid()

	hostname, err := os.Hostname()
	if err != nil || hostname == "" {
		hostname = "localhost"
	}

	var acc int
	for i := 0; i < len(hostname); i++ {
		acc += int(hostname[i])
	}
	hostID := len(hostname) + 36 + acc

	return padLeft(base36(uint64(pid), 2), 2)[:2] + padLeft(base36(uint64(hostID), 2), 2)[:2]
}

func randomBlock() string {
	var b [4]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Fall back to time-derived randomness; extremely unlikely path.
		binary.LittleEndian.PutUint32(b[:], uint32(time.Now().UnixNano()))
	}
	return padLeft(base36(uint64(binary.BigEndian.Uint32(b[:])), cuidBlockSize), cuidBlockSize)
}

// GenerateID returns a new cuid-compatible identifier. Exported for tools
// (e.g. cmd/createuser) that insert rows outside the model methods.
func GenerateID() string {
	return newID()
}

// newID returns a new cuid-compatible identifier.
func newID() string {
	timestamp := base36(uint64(time.Now().UnixMilli()), 8)
	counter := atomic.AddUint32(&cuidCounter, 1)
	count := padLeft(base36(uint64(counter)%(36*36*36*36), cuidBlockSize), cuidBlockSize)
	random := randomBlock() + randomBlock()

	return fmt.Sprintf("c%s%s%s%s", timestamp, count, cuidFingerprint, random)
}
