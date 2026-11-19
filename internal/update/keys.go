package update

// publicKeys are the Ed25519 keys that sign releases' checksums.txt, base64
// encoded. A release is only installed if one of them signed it. To rotate
// the key, add the new one here, ship a release, then sign with the new key.
// The private half lives in the LEFTOVER_UPDATE_KEY secret of the release
// workflow; see tools/sign.
var publicKeys = []string{
	"6j3BXY5rLIBH41qywf1ART9EXcB7hERA6+5/WZ8ghdI=",
}
