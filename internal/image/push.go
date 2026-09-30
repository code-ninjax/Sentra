package image

import (
	"errors"
	"fmt"
	"os"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// PushResult describes a completed push.
type PushResult struct {
	Reference string // fully-qualified destination
	Digest    string // manifest digest the registry stored
	Layers    int    // number of build layers uploaded
}

// PushResult carries a completed push's outcome.

// Assemble builds the OCI image for a push: the registry's copy of the base
// image with this build's layers appended and the image config applied.
//
// It is separated from the network write so the assembled artifact can be
// validated without a registry — the same v1.Image value is what remote.Write
// uploads.
func Assemble(baseRef string, layerTarballs []string, cfg *v1.ConfigFile) (v1.Image, error) {
	base, err := baseImage(baseRef)
	if err != nil {
		return nil, err
	}

	layers := make([]v1.Layer, 0, len(layerTarballs))
	for _, path := range layerTarballs {
		layer, err := tarball.LayerFromFile(path)
		if err != nil {
			return nil, fmt.Errorf("open layer tarball %s: %w", path, err)
		}
		layers = append(layers, layer)
	}

	img, err := mutate.AppendLayers(base, layers...)
	if err != nil {
		return nil, fmt.Errorf("assemble image for push: %w", err)
	}

	if cfg != nil {
		img, err = applyOverrides(img, cfg)
		if err != nil {
			return nil, err
		}
	}
	return img, nil
}

// applyOverrides folds the Sentrafile's env, ports and entrypoint onto the
// config the layers already produced.
//
// mutate.ConfigFile replaces the config wholesale, which would discard
// rootfs.diff_ids and leave an image no runtime can start. Only the fields
// Sentra actually owns are replaced; the diff IDs stay as computed.
func applyOverrides(img v1.Image, override *v1.ConfigFile) (v1.Image, error) {
	cfg, err := img.ConfigFile()
	if err != nil {
		return nil, fmt.Errorf("read assembled config: %w", err)
	}
	if cfg == nil {
		return nil, errors.New("assembled image has no config file")
	}

	if len(override.Config.Entrypoint) > 0 {
		cfg.Config.Entrypoint = override.Config.Entrypoint
	}
	if len(override.Config.Env) > 0 {
		cfg.Config.Env = override.Config.Env
	}
	if len(override.Config.ExposedPorts) > 0 {
		cfg.Config.ExposedPorts = override.Config.ExposedPorts
	}

	out, err := mutate.ConfigFile(img, cfg)
	if err != nil {
		return nil, fmt.Errorf("apply image config: %w", err)
	}
	return out, nil
}

// baseImage reads the base image the build started from.
func baseImage(baseRef string) (v1.Image, error) {
	// The base is read anonymously even when pushing authenticated: it came
	// from a public registry, and only the destination needs a token.
	ref, err := name.ParseReference(baseRef)
	if err != nil {
		return nil, fmt.Errorf("invalid base reference %q: %w", baseRef, err)
	}
	img, err := remote.Image(ref, remote.WithUserAgent("sentra/0.1"))
	if err != nil {
		return nil, fmt.Errorf("fetch base %s for push: %w", baseRef, err)
	}
	return img, nil
}

// Push uploads a built image.
//
// Only the build's own layers are uploaded: the base image is referenced
// from the registry it was pulled from, so a Sentrafile that changes one
// file uploads one small layer rather than the whole image. That is the
// same reason the build engine records per-step diffs.
//
// Auth: anonymous by default, which is enough for public registries that
// allow anonymous writes. A token can be supplied through
// SENTRA_REGISTRY_TOKEN; credential storage is deliberately not built yet.
func Push(destRef, baseRef string, layerTarballs []string, cfg *v1.ConfigFile) (*PushResult, error) {
	dest, err := name.ParseReference(destRef)
	if err != nil {
		return nil, fmt.Errorf("invalid destination reference %q: %w", destRef, err)
	}

	img, err := Assemble(baseRef, layerTarballs, cfg)
	if err != nil {
		return nil, err
	}

	opts := []remote.Option{remote.WithUserAgent("sentra/0.1")}
	if auth := authForPush(); auth != nil {
		opts = append(opts, remote.WithAuth(auth))
	}

	if err := remote.Write(dest, img, opts...); err != nil {
		return nil, fmt.Errorf("push to %s: %w", dest.Name(), err)
	}

	digest, err := img.Digest()
	if err != nil {
		return nil, err
	}
	return &PushResult{
		Reference: dest.Name(),
		Digest:    digest.String(),
		Layers:    len(layerTarballs),
	}, nil
}

// authForPush returns an authenticator when a token was supplied, and nil
// for anonymous access otherwise.
func authForPush() authn.Authenticator {
	token := os.Getenv("SENTRA_REGISTRY_TOKEN")
	if token == "" {
		return nil
	}
	return authn.FromConfig(authn.AuthConfig{RegistryToken: token})
}
