package idlib

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// DefaultNamespace is uuid.NameSpaceDNS matching Python toolset uuid5_from_args defaults.
var DefaultNamespace = uuid.NameSpaceDNS

// UUID5FromArgs computes a deterministic RFC 4122 UUIDv5 string from namespace and arbitrary arguments.
// Passing uuid.Nil defaults to uuid.NameSpaceDNS.
func UUID5FromArgs(namespace uuid.UUID, args ...any) string {
	if namespace == uuid.Nil {
		namespace = DefaultNamespace
	}
	parts := make([]string, len(args))
	for i, a := range args {
		parts[i] = fmt.Sprint(a)
	}
	name := strings.Join(parts, "|")
	return uuid.NewSHA1(namespace, []byte(name)).String()
}

// UUID5FromStrings computes a deterministic UUIDv5 string using DefaultNamespace.
func UUID5FromStrings(parts ...string) string {
	return uuid.NewSHA1(DefaultNamespace, []byte(strings.Join(parts, "|"))).String()
}

// NewUUID4 generates a new random RFC 4122 UUIDv4 string.
func NewUUID4() string {
	return uuid.NewString()
}
