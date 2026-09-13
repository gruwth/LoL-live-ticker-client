package updater

import (
	"crypto/ed25519"
	"encoding/base64"
)

// ReleasePage is where a user goes when an update could not be applied.
const ReleasePage = "https://github.com/gruwth/LoL-live-ticker-client/releases/latest"

// ReleaseURL is where the updater fetches this platform's binary.
var ReleaseURL = "https://github.com/gruwth/LoL-live-ticker-client/releases/latest/download/" + AssetName()

// publicKeyB64 is the ed25519 public key that release binaries are signed
// with. The matching private key lives only in GitHub Actions secrets and
// never in this repository.
//
// IT IS EMPTY ON PURPOSE. No keypair has been generated yet, and embedding a
// placeholder would mean shipping an updater that either rejects every real
// update or, worse, looks like it is verifying something when it is not. With
// this empty, New returns nil and the app has no updater at all, which is the
// honest and safe state.
//
// To finish this:
//
//	go run github.com/fynelabs/selfupdate/cmd/selfupdatectl@latest create-keys
//
// then put the private key in the repository secret UPDATE_PRIVATE_KEY (the
// release workflow already reads it) and paste the public key below.
const publicKeyB64 = ""

// PublicKey decodes the embedded key, or returns nil if there is none.
func PublicKey() ed25519.PublicKey {
	if publicKeyB64 == "" {
		return nil
	}
	b, err := base64.StdEncoding.DecodeString(publicKeyB64)
	if err != nil || len(b) != ed25519.PublicKeySize {
		return nil
	}
	return ed25519.PublicKey(b)
}
