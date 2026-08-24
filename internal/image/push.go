package image

import "errors"

// Push uploads an image to a registry. Stubbed in workstream 2 — pull is
// the MVP deliverable. Real push lands after the build engine (workstream
// 4) exists, since there's nothing worth pushing until then; it will use
// go-containerregistry's remote.Write against a v1.Image built from the
// layer store.
//
// TODO(workstream 4+): implement via remote.Write(ref, img,
// remote.WithAuth(keychain)).
func Push(refStr string) error {
	_ = refStr
	return errors.New("sentra push: not implemented yet (planned after the build engine, workstream 4)")
}
