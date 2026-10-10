package auth_test

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/go-webauthn/webauthn/protocol"
	"github.com/go-webauthn/webauthn/protocol/webauthncbor"
	"github.com/go-webauthn/webauthn/protocol/webauthncose"
	"github.com/go-webauthn/webauthn/webauthn"

	"ft/internal/auth"
	. "ft/internal/testutil"
)

// softKey is a software authenticator: one P-256 passkey for finance.example.
type softKey struct {
	id  []byte
	key *ecdsa.PrivateKey
}

func newSoftKey(t *testing.T) softKey {
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	return softKey{id: []byte("mac-passkey-1"), key: k}
}

// record is the stored credential, enrolled with the given backup-eligible flag.
func (k softKey) record(t *testing.T, backupEligible bool) []byte {
	pub, err := webauthncbor.Marshal(webauthncose.EC2PublicKeyData{
		PublicKeyData: webauthncose.PublicKeyData{KeyType: int64(webauthncose.EllipticKey), Algorithm: int64(webauthncose.AlgES256)},
		Curve:         1, XCoord: k.key.X.FillBytes(make([]byte, 32)), YCoord: k.key.Y.FillBytes(make([]byte, 32)),
	})
	if err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(webauthn.Credential{ID: k.id, PublicKey: pub, Flags: webauthn.CredentialFlags{UserPresent: true, UserVerified: true, BackupEligible: backupEligible}})
	return b
}

// assert answers a login challenge; flags are the authenticator-data flags.
func (k softKey) assert(t *testing.T, challenge string, flags byte, count uint32, tamper bool) *bytes.Reader {
	rpHash := sha256.Sum256([]byte("finance.example"))
	authData := append(rpHash[:], flags)
	authData = binary.BigEndian.AppendUint32(authData, count)
	client, _ := json.Marshal(map[string]string{"type": "webauthn.get", "challenge": challenge, "origin": "https://finance.example:8443"})
	ch := sha256.Sum256(client)
	digest := sha256.Sum256(append(append([]byte{}, authData...), ch[:]...))
	sig, err := ecdsa.SignASN1(rand.Reader, k.key, digest[:])
	if err != nil {
		t.Fatal(err)
	}
	if tamper {
		sig[len(sig)-1] ^= 0xff
	}
	b64 := base64.RawURLEncoding.EncodeToString
	body, _ := json.Marshal(map[string]any{"id": b64(k.id), "rawId": b64(k.id), "type": "public-key",
		"response": map[string]any{"authenticatorData": b64(authData), "clientDataJSON": b64(client), "signature": b64(sig)}})
	return bytes.NewReader(body)
}

const (
	flagUP = 0x01
	flagUV = 0x04
	flagBE = 0x08
	flagBS = 0x10
)

// A passkey enrolled as device-bound that now reports itself synced (a Mac's
// passkey moved into iCloud Keychain) still unlocks — the signature decides —
// and the new flag is kept. A forged signature never unlocks.
func TestPasskeyBackupFlagDrift(t *testing.T) {
	d := DB(t)
	k := newSoftKey(t)
	d.Exec(`INSERT INTO webauthn_credentials(name,credential,created_at) VALUES('Work Macbook',?, 'x')`, k.record(t, false))
	s := &auth.Service{DB: d}
	login := func(flags byte, count uint32, tamper bool) error {
		opts, err := s.BeginLogin("finance.example:8443", "https://finance.example:8443")
		if err != nil {
			t.Fatal(err)
		}
		allow := opts.Response.AllowedCredentials
		if len(allow) != 1 || len(allow[0].Transport) != 2 || allow[0].Transport[0] != protocol.Internal {
			t.Fatalf("an old passkey offers this device and a phone: %+v", allow)
		}
		r := httptest.NewRequest("POST", "/api/auth/passkey/login/finish", k.assert(t, opts.Response.Challenge.String(), flags, count, tamper))
		_, err = s.FinishLogin("finance.example:8443", "https://finance.example:8443", r)
		return err
	}
	if err := login(flagUP|flagUV|flagBE|flagBS, 1, true); err == nil {
		t.Fatal("forged signature unlocked")
	}
	if err := login(flagUP|flagUV|flagBE|flagBS, 2, false); err != nil {
		t.Fatalf("synced passkey refused: %v", err)
	}
	var blob []byte
	d.QueryRow(`SELECT credential FROM webauthn_credentials`).Scan(&blob)
	var c webauthn.Credential
	json.Unmarshal(blob, &c)
	if !c.Flags.BackupEligible {
		t.Error("new backup-eligible flag not kept")
	}
	if err := login(flagUP|flagUV|flagBE|flagBS, 3, false); err != nil {
		t.Fatalf("second login: %v", err)
	}
	if err := login(flagUP|flagBE|flagBS, 4, false); err == nil {
		t.Fatal("no fingerprint (user verification) unlocked")
	}
}
