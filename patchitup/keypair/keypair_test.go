package keypair

import (
	crypto_rand "crypto/rand"
	"encoding/json"
	"fmt"
	"testing"

	"golang.org/x/crypto/nacl/box"

	"github.com/stretchr/testify/assert"
)

// func BenchmarkEncrypt(b *testing.B) {
// 	bob, _ := New()
// 	jane, _ := New()
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		_, err := bob.Encrypt([]byte(`hello, world. this, is 32 bytes!`), jane)
// 		if err != nil {
// 			panic(err)
// 		}
// 	}
// }
// func BenchmarkDecrypt(b *testing.B) {
// 	bob, _ := New()
// 	jane, _ := New()
// 	enc, err := bob.Encrypt([]byte(`hello, world. this, is 32 bytes!`), jane)
// 	if err != nil {
// 		panic(err)
// 	}
// 	b.ResetTimer()
// 	for i := 0; i < b.N; i++ {
// 		_, err := jane.Decrypt(enc, bob)
// 		if err != nil {
// 			panic(err)
// 		}
// 	}
// }
func TestKeyPairEncryption(t *testing.T) {
	bob := New()
	jane := New()
	enc, err := bob.Encrypt([]byte(`hello, world`), jane)
	assert.Nil(t, err)
	dec, err := jane.Decrypt(enc, bob)
	assert.Nil(t, err)
	assert.Equal(t, []byte(`hello, world`), dec)
	dec, err = bob.Decrypt(enc, bob)
	assert.NotNil(t, err)
	assert.NotEqual(t, []byte(`hello, world`), dec)
}

func TestKeyPairs(t *testing.T) {
	sendPublicKeyString, senderPrivateKeyString := GenerateKeys()
	senderPublicKey, err := keyStringToBytes(sendPublicKeyString)
	assert.Nil(t, err)
	senderPrivateKey, _ := keyStringToBytes(senderPrivateKeyString)
	recipientPublicKey, recipientPrivateKey, err := box.GenerateKey(crypto_rand.Reader)
	if err != nil {
		panic(err)
	}

	enc, err := encryptWithKeyPair([]byte(`hello world`), senderPrivateKey, recipientPublicKey)
	assert.Nil(t, err)
	dec, err := decryptWithKeyPair(enc, senderPublicKey, recipientPrivateKey)
	assert.Nil(t, err)
	assert.Equal(t, "hello world", string(dec))
}

func TestMarshaling(t *testing.T) {
	kp := New()
	kpMarshaled, err := json.Marshal(kp)
	assert.Nil(t, err)

	var kp2 *KeyPair
	err = json.Unmarshal(kpMarshaled, &kp2)
	assert.Nil(t, err)
	assert.Equal(t, kp.private, kp2.private)
	assert.Equal(t, kp.public, kp2.public)
	assert.Equal(t, kp.Public, kp2.Public)
	assert.Equal(t, kp.Private, kp2.Private)

	// test just having a public key
	kp3, err := FromPublic(kp.Public)
	assert.Nil(t, err)
	kpMarshaled, err = json.Marshal(kp3)
	assert.Nil(t, err)

	var kp4 *KeyPair
	err = json.Unmarshal(kpMarshaled, &kp4)
	assert.Nil(t, err)
	assert.Equal(t, kp3.Public, kp4.Public)
}

func TestStringEncode(t *testing.T) {
	a := New()
	fmt.Println(a.Hash())
}
func TestSigning(t *testing.T) {
	shared := New()
	bob := New()
	jane := New()

	// bob generates a signature
	signature, err := bob.Signature(shared)
	assert.Nil(t, err)
	fmt.Println("signature:", signature)
	signature, err = bob.Signature(shared)
	assert.Nil(t, err)
	fmt.Println("signature:", signature)

	// someone has bob's public address
	bobsPublicAddress := bob.Public
	bobKeyPairFromPublic, err := FromPublic(bobsPublicAddress)
	fmt.Println(bobKeyPairFromPublic)
	assert.Nil(t, err)
	err = shared.Validate(signature, bobKeyPairFromPublic)
	assert.Nil(t, err)

	err = shared.Validate(signature, jane)
	assert.NotNil(t, err)
}

func TestDeterminsitcGeneration(t *testing.T) {
	assert.Equal(t, NewDeterministic("123"), NewDeterministic("123"))
	assert.NotEqual(t, NewDeterministic("123"), NewDeterministic("124"))
}
