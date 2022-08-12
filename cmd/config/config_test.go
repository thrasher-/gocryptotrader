package main

import "testing"

func TestEncryptOrDecrypt(t *testing.T) {
	t.Parallel()
	reValue := EncryptOrDecrypt(true)
	if reValue != "encrypted" {
		t.Error(
			"expected encrypted",
		)
	}
	reValue = EncryptOrDecrypt(false)
	if reValue != "decrypted" {
		t.Error(
			"expected decrypted",
		)
	}
}
