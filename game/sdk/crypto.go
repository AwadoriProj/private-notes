package sdk

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"os"
)

type RSAKeys struct {
	Private *rsa.PrivateKey
	PEM     string
	Hash    string
}

func LoadOrCreateRSAKeys(path string) (*RSAKeys, error) {
	if data, err := os.ReadFile(path); err == nil {
		block, _ := pem.Decode(data)
		if block == nil {
			return nil, fmt.Errorf("invalid pem at %s", path)
		}
		priv, err := x509.ParsePKCS1PrivateKey(block.Bytes)
		if err != nil {
			return nil, err
		}
		return keysFromPrivate(priv)
	}

	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}
	block := &pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(priv)}
	if err := os.WriteFile(path, pem.EncodeToMemory(block), 0600); err != nil {
		return nil, err
	}
	return keysFromPrivate(priv)
}

func keysFromPrivate(priv *rsa.PrivateKey) (*RSAKeys, error) {
	pubBytes, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, err
	}
	pubPEM := pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: pubBytes})
	sum := sha256.Sum256(pubBytes)
	return &RSAKeys{
		Private: priv,
		PEM:     string(pubPEM),
		Hash:    fmt.Sprintf("%x", sum[:8]),
	}, nil
}

func (k *RSAKeys) DecryptEmail(b64 string) (string, error) {
	cipherBytes, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return "", err
	}
	plain, err := rsa.DecryptPKCS1v15(rand.Reader, k.Private, cipherBytes)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func b64url(b []byte) string {
	return base64.RawURLEncoding.EncodeToString(b)
}

func (k *RSAKeys) SignJWT(claims interface{}) (string, error) {
	headerJSON := []byte(`{"alg":"RS256"}`)
	claimsJSON, _ := json.Marshal(claims)
	signingInput := b64url(headerJSON) + "." + b64url(claimsJSON)

	digest := sha256.Sum256([]byte(signingInput))
	sig, err := rsa.SignPKCS1v15(rand.Reader, k.Private, crypto.SHA256, digest[:])
	if err != nil {
		return "", err
	}
	return signingInput + "." + b64url(sig), nil
}