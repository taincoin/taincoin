package wallet

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"encoding/gob"
	"log"

	"github.com/taincoin/taincoin/lib"
	"github.com/taincoin/taincoin/lib/utils"
)

const keysTestString = "this is the test string to use for new keys. to know that sign and verify works fine"

// Wallet stores private and public keys
type Wallet struct {
	PrivateKey ecdsa.PrivateKey
	PublicKey  []byte
}

type WalletBalance struct {
	Total    float64
	Approved float64
	Pending  float64
}

// MakeWallet creates Wallet. It generates new keys pair and assign to the object
func (w *Wallet) MakeWallet() {
	var private ecdsa.PrivateKey
	var public []byte

	i := 0

	for {
		if i > 1000 {
			// we were not able to find good keys in 1000 attempts.
			// somethign must be very wrong here
			break
		}
		private, public = w.newKeyPair()

		signature, err := utils.SignData(private, []byte(keysTestString))

		i++

		if err != nil {
			continue
		}

		vr, err := utils.VerifySignature(signature, []byte(keysTestString), public)

		if err != nil {
			continue
		}

		if vr {
			break
		}
	}

	w.PrivateKey = private
	w.PublicKey = public
}

// Returns public key of a wallet
func (w Wallet) GetPublicKey() []byte {
	return w.PublicKey
}

// Reurns private key of a wallet
func (w Wallet) GetPrivateKey() ecdsa.PrivateKey {
	return w.PrivateKey
}

// GetAddress returns wallet address
func (w Wallet) GetAddress() []byte {
	pubKeyHash, _ := utils.HashPubKey(w.PublicKey)

	versionedPayload := append([]byte{lib.Version}, pubKeyHash...)
	checksum := utils.Checksum(versionedPayload)

	fullPayload := append(versionedPayload, checksum...)
	address := utils.Base58Encode(fullPayload)

	return address
}

// ValidateAddress check if address is valid, has valid format
func (w Wallet) ValidateAddress(address string) bool {
	if len(address) == 0 {
		return false
	}

	pubKeyHash := utils.Base58Decode([]byte(address))

	if len(pubKeyHash) <= lib.AddressChecksumLen {
		return false
	}
	actualChecksum := pubKeyHash[len(pubKeyHash)-lib.AddressChecksumLen:]
	version := pubKeyHash[0]
	pubKeyHash = pubKeyHash[1 : len(pubKeyHash)-lib.AddressChecksumLen]
	targetChecksum := utils.Checksum(append([]byte{version}, pubKeyHash...))

	return bytes.Compare(actualChecksum, targetChecksum) == 0
}

// Generate new key pair to create new wallet
func (w *Wallet) newKeyPair() (ecdsa.PrivateKey, []byte) {
	curve := elliptic.P256()
	private, err := ecdsa.GenerateKey(curve, rand.Reader)
	if err != nil {
		log.Panic(err)
	}
	pubKey := append(private.PublicKey.X.Bytes(), private.PublicKey.Y.Bytes()...)

	return *private, pubKey
}
func (w Wallet) Serialize() ([]byte, error) {
	var encoded bytes.Buffer

	enc := gob.NewEncoder(&encoded)
	err := enc.Encode(w)

	if err != nil {
		return nil, err
	}

	return encoded.Bytes(), nil
}
func (w *Wallet) Deserialize(data []byte) error {
	decoder := gob.NewDecoder(bytes.NewReader(data))
	err := decoder.Decode(w)

	if err != nil {
		return err
	}

	return nil
}
