package image

import (
	"fmt"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

// Registry is a thin client over go-containerregistry. All manifest/config/
// blob HTTP work is delegated to it — Sentra never hand-rolls Registry API
// calls.
//
// Auth note: pulls are anonymous for now. The hookup point is the
// remote.WithAuth option in Image() — swap authn.Anonymous for
// authn.DefaultKeychain when login lands (post-MVP; do not build custom
// credential storage before then).
type Registry struct{}

func NewRegistry() *Registry { return &Registry{} }

// ParseRef resolves a user-supplied reference like "alpine",
// "alpine:3.20", "ghcr.io/owner/img:tag" into a fully-qualified reference.
// Bare names default to index.docker.io + :latest.
func ParseRef(refStr string) (name.Reference, error) {
	ref, err := name.ParseReference(refStr)
	if err != nil {
		return nil, fmt.Errorf("invalid image reference %q: %w", refStr, err)
	}
	return ref, nil
}

// Image resolves refStr and returns a lazy v1.Image handle. Blobs download
// only when pull.go asks for them.
func (r *Registry) Image(refStr string) (name.Reference, v1.Image, error) {
	ref, err := ParseRef(refStr)
	if err != nil {
		return nil, nil, err
	}
	img, err := remote.Image(ref,
		remote.WithAuth(authn.Anonymous),
		remote.WithUserAgent("sentra/0.1"),
	)
	if err != nil {
		return ref, nil, fmt.Errorf("resolve %s: %w", ref.Name(), err)
	}
	return ref, img, nil
}
