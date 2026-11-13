//go:build windows

package main

// updatePublicKey is the publisher's key: updates are accepted only when signed with its
// private half (written by `release keygen`, kept outside the repository).
const updatePublicKey = "9ad037f6dfb756ef0ad08eea329891d9b5d0b2c5c60929f41eb5561955439cb9"
