package keypair

import (
	"bytes"
	crypto_rand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"io"
	math_rand "math/rand"
	"strings"

	"github.com/mr-tron/base58/base58"
	"github.com/pkg/errors"
	"github.com/schollz/mnemonicode"
	"golang.org/x/crypto/nacl/box"
)

type KeyPair struct {
	Public  string `json:"public"`
	Private string `json:"private,omitempty"`
	private *[32]byte
	public  *[32]byte
}

func New() (kp KeyPair) {
	var err error
	kp = KeyPair{}
	kp.Public, kp.Private = GenerateKeys()
	kp.public, err = keyStringToBytes(kp.Public)
	if err != nil {
		panic(err)
	}
	kp.private, err = keyStringToBytes(kp.Private)
	if err != nil {
		panic(err)
	}
	return
}

func (kp KeyPair) Hash() string {
	result := []string{}
	h := fnv.New32a()
	h.Write(kp.public[:])
	h.Write(kp.private[:])
	bs := make([]byte, 4)
	binary.LittleEndian.PutUint32(bs, h.Sum32())
	result = mnemonicode.EncodeWordList(result, bs)
	return strings.Join(result, "-")
}

func (kp KeyPair) PublicKey() (kpPublic KeyPair) {
	var err error
	if kp.Public == "" {
		panic(errors.New("has blank key!"))
	}
	kpPublic = KeyPair{}
	kpPublic.Public = kp.Public
	kpPublic.public, err = keyStringToBytes(kpPublic.Public)
	if err != nil {
		panic(err)
	}
	return
}

func FromPair(public, private string) (kp KeyPair, err error) {
	kp = KeyPair{}
	kp.Public, kp.Private = public, private
	kp.public, err = keyStringToBytes(kp.Public)
	if err != nil {
		return
	}
	kp.private, err = keyStringToBytes(kp.Private)
	if err != nil {
		return
	}
	return
}

// FromPublic generates a half-key pair
func FromPublic(public string) (kp KeyPair, err error) {
	kp = KeyPair{}
	kp.Public = public
	kp.public, err = keyStringToBytes(kp.Public)
	if err != nil {
		return
	}
	return
}

func (kp KeyPair) MarshalJSON() ([]byte, error) {
	buffer := bytes.NewBufferString("{")
	if kp.Private == "" {
		buffer.WriteString(fmt.Sprintf("\"%s\":\"%s\"", "public", kp.Public))
	} else {
		buffer.WriteString(fmt.Sprintf("\"%s\":\"%s\",", "public", kp.Public))
		buffer.WriteString(fmt.Sprintf("\"%s\":\"%s\"", "private", kp.Private))
	}
	buffer.WriteString("}")
	return buffer.Bytes(), nil
}

type KeyPairJSON struct {
	Public  string
	Private string
}

func (kp *KeyPair) UnmarshalJSON(b []byte) (err error) {
	var kpBase KeyPairJSON
	err = json.Unmarshal(b, &kpBase)
	if err != nil {
		return
	}
	kp.Public = kpBase.Public
	kp.Private = kpBase.Private
	if len(kpBase.Public) > 0 {
		kp.public, err = keyStringToBytes(kpBase.Public)
		if err != nil {
			return
		}
	}
	if len(kpBase.Private) > 0 {
		kp.private, err = keyStringToBytes(kpBase.Private)
		if err != nil {
			return
		}
	}
	return
}

func (kp KeyPair) Encrypt(msg []byte, recipient KeyPair) (encrypted []byte, err error) {
	encrypted, err = encryptWithKeyPair(msg, kp.private, recipient.public)
	return
}

func (kp KeyPair) Decrypt(encrypted []byte, sender KeyPair) (msg []byte, err error) {
	msg, err = decryptWithKeyPair(encrypted, sender.public, kp.private)
	return
}

func GenerateKeysDeterministic(seedBytes []byte) (publicKey, privateKey string) {
	h := fnv.New32a()
	h.Write(seedBytes)
	math_rand.Seed(int64(h.Sum32()))
	b := make([]byte, 512)
	math_rand.Read(b)
	reader := bytes.NewReader(b)
	publicKeyBytes, privateKeyBytes, err := box.GenerateKey(reader)
	if err != nil {
		panic(err)
	}

	publicKey = base58.FastBase58Encoding(publicKeyBytes[:])
	privateKey = base58.FastBase58Encoding(privateKeyBytes[:])
	return
}

func NewDeterministic(passphrase string) (kp KeyPair) {
	pub, priv := GenerateKeysDeterministic([]byte(passphrase))
	kp, _ = FromPair(pub, priv)
	return
}

func GenerateKeys() (publicKey, privateKey string) {
	publicKeyBytes, privateKeyBytes, err := box.GenerateKey(crypto_rand.Reader)
	if err != nil {
		panic(err)
	}

	publicKey = base58.FastBase58Encoding(publicKeyBytes[:])
	privateKey = base58.FastBase58Encoding(privateKeyBytes[:])
	return
}

func keyStringToBytes(s string) (key *[32]byte, err error) {
	keyBytes, err := base58.FastBase58Decoding(s)
	if err != nil {
		return
	}
	key = new([32]byte)
	copy(key[:], keyBytes[:32])
	return
}

func encryptWithKeyPair(msg []byte, senderPrivateKey, recipientPublicKey *[32]byte) (encrypted []byte, err error) {
	// You must use a different nonce for each message you encrypt with the
	// same key. Since the nonce here is 192 bits long, a random value
	// provides a sufficiently small probability of repeats.
	var nonce [24]byte
	if _, err = io.ReadFull(crypto_rand.Reader, nonce[:]); err != nil {
		return
	}
	// This encrypts msg and appends the result to the nonce.
	encrypted = box.Seal(nonce[:], msg, &nonce, recipientPublicKey, senderPrivateKey)
	return
}

func decryptWithKeyPair(enc []byte, senderPublicKey, recipientPrivateKey *[32]byte) (decrypted []byte, err error) {
	// The recipient can decrypt the message using their private key and the
	// sender's public key. When you decrypt, you must use the same nonce you
	// used to encrypt the message. One way to achieve this is to store the
	// nonce alongside the encrypted message. Above, we stored the nonce in the
	// first 24 bytes of the encrypted text.
	var decryptNonce [24]byte
	copy(decryptNonce[:], enc[:24])
	var ok bool
	decrypted, ok = box.Open(nil, enc[24:], &decryptNonce, senderPublicKey, recipientPrivateKey)
	if !ok {
		err = errors.New("keypair decryption failed")
	}
	return
}

// Signature returns your public key encrypted by a shared region key. Anyone who has the shared region key can decrypt this and see that the contents do indeed match that public key used to make it.
func (kp KeyPair) Signature(regionkey KeyPair) (signature string, err error) {
	encrypted, err := kp.Encrypt([]byte(kp.Public), regionkey)
	if err != nil {
		return
	}
	signature = base58.FastBase58Encoding(encrypted)
	return
}

// Validate using the specified keypair (usually should be shared region key)
func (kp KeyPair) Validate(signature string, sender KeyPair) (err error) {
	if signature == "" {
		return errors.New("no signature to validate")
	}
	if sender.Public == "" {
		return errors.New("no public key to validate")
	}
	encryptedPublicKey, err := base58.FastBase58Decoding(signature)
	if err != nil {
		return errors.Wrap(err, "not base58")
	}
	decryptedPublicKey, err := kp.Decrypt(encryptedPublicKey, sender)
	if err != nil {
		return errors.Wrap(err, "not decryptable")
	}
	if string(decryptedPublicKey) != sender.Public {
		return errors.New("signature corrupted")
	}
	return
}
